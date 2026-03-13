package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
)

type WalletService interface {
	SyncSubscriptions() error
	CreateWallet(ctx context.Context, req models.WalletCreateRequest) (*models.WalletCreateResponse, error)
	QueryWalletInfo(ctx context.Context, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error)
	QueryTransferOutAssets(ctx context.Context, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error)
	TransferOut(ctx context.Context, req models.TransferOutRequest) (*models.TransferOutResponse, error)
	QueryTransaction(ctx context.Context, req models.TransactionQueryRequest) (*models.TransactionQueryResponse, error)
	QueryHistory(ctx context.Context, req models.TransactionHistoryQueryRequest) (*models.TransactionHistoryQueryResponse, error)
	StartMQConsumer() error
	HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error
	HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error
}

func NewWalletService(cfg *config.Config, st store.Store, httpClient *http.Client) WalletService {
	return &walletService{
		cfg:        cfg,
		store:      st,
		httpClient: httpClient,
	}
}

type walletService struct {
	cfg        *config.Config
	store      store.Store
	httpClient *http.Client
	mqOnce     sync.Once
}

func (s *walletService) SyncSubscriptions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wallets, err := s.store.ListActiveWallets(ctx, s.networkCode())
	if err != nil {
		return err
	}
	for _, wallet := range wallets {
		if !wallet.DepositEnabled {
			continue
		}
		if err := s.connectorPost(ctx, "/api/v1/inner/chain-data-subscribe/solana/address-subscribe", map[string]string{
			"address": wallet.Address,
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *walletService) CreateWallet(ctx context.Context, req models.WalletCreateRequest) (*models.WalletCreateResponse, error) {
	network := s.networkCode()
	walletNo := generateWalletNo()
	password := generatePassword()

	var keyStoreResp struct {
		KeystoreId string `json:"keystore_id"`
	}
	if err := s.kmsPost(ctx, "/kms/keystore/create", map[string]string{
		"password": password,
	}, &keyStoreResp); err != nil {
		return nil, wrapSystemError(err)
	}

	var addressResp []struct {
		Network string `json:"chain_type"`
		Address string `json:"address"`
	}
	if err := s.kmsPost(ctx, "/kms/keystore/address/0/0/0", map[string]string{
		"keystore_id": keyStoreResp.KeystoreId,
		"password":    password,
	}, &addressResp); err != nil {
		var wrappedAddressResp struct {
			KeyAddress []struct {
				Network string `json:"chain_type"`
				Address string `json:"address"`
			} `json:"key_address"`
		}
		if err = s.kmsPost(ctx, "/kms/keystore/address/0/0/0", map[string]string{
			"keystore_id": keyStoreResp.KeystoreId,
			"password":    password,
		}, &wrappedAddressResp); err != nil {
			return nil, wrapSystemError(err)
		}
		addressResp = wrappedAddressResp.KeyAddress
	}

	address := ""
	for _, item := range addressResp {
		if strings.EqualFold(item.Network, network) {
			address = item.Address
			break
		}
	}
	if address == "" {
		return nil, newAppError(models.CodeSystemBusy, "failed to derive wallet address")
	}

	wallet := &models.WalletEntity{
		WalletNo:        walletNo,
		Network:         network,
		Address:         address,
		KMSKeystoreID:   keyStoreResp.KeystoreId,
		KMSPassword:     password,
		KMSKeyType:      "mnemonic",
		AccountIndex:    "0",
		ChangeIndex:     "0",
		AddressIndex:    "0",
		DepositEnabled:  true,
		TransferEnabled: true,
		Status:          "ACTIVE",
	}
	if err := s.store.CreateWallet(ctx, wallet); err != nil {
		return nil, wrapSystemError(err)
	}

	if err := s.connectorPost(ctx, "/api/v1/inner/chain-data-subscribe/solana/address-subscribe", map[string]string{
		"address": address,
	}, nil); err != nil {
		return nil, wrapSystemError(err)
	}

	return &models.WalletCreateResponse{
		WalletNo:   wallet.WalletNo,
		Network:    wallet.Network,
		Address:    wallet.Address,
		KeystoreID: wallet.KMSKeystoreID,
	}, nil
}

func (s *walletService) QueryWalletInfo(ctx context.Context, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error) {
	wallet, err := s.getWallet(ctx, req.WalletNo)
	if err != nil {
		return nil, err
	}

	items := make([]models.WalletTokenBalance, 0, 8)
	var nativeResp struct {
		Balance     float64 `json:"balance"`
		BalanceUnit string  `json:"balanceUnit"`
	}
	if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/address-balance", map[string]string{
		"address": wallet.Address,
	}, &nativeResp); err != nil {
		return nil, wrapSystemError(err)
	}
	items = append(items, models.WalletTokenBalance{
		TokenSymbol: s.nativeTokenSymbol(),
		Balance:     formatFloat(nativeResp.Balance),
	})

	tokens, err := s.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		var tokenResp struct {
			Value float64 `json:"value"`
		}
		if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/token-balance", map[string]string{
			"address":   wallet.Address,
			"tokenCode": token.Code,
		}, &tokenResp); err != nil {
			return nil, wrapSystemError(err)
		}
		items = append(items, models.WalletTokenBalance{
			TokenSymbol: token.Code,
			Balance:     formatFloat(tokenResp.Value),
		})
	}

	return &models.WalletInfoQueryResponse{
		WalletNo: wallet.WalletNo,
		Tokens:   items,
	}, nil
}

func (s *walletService) QueryTransferOutAssets(ctx context.Context, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error) {
	wallet, err := s.getWallet(ctx, req.WalletNo)
	if err != nil {
		return nil, err
	}
	if !wallet.TransferEnabled {
		return nil, newAppError(models.CodePermissionDenied, "wallet transfer is disabled")
	}

	assets := []models.TransferableAsset{
		{
			Network:      wallet.Network,
			TokenAddress: models.TokenNative,
			TokenSymbol:  s.nativeTokenSymbol(),
		},
	}
	tokens, err := s.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		assets = append(assets, models.TransferableAsset{
			Network:      wallet.Network,
			TokenAddress: token.MintAddress,
			TokenSymbol:  token.Code,
		})
	}
	return &models.TransferOutQueryResponse{
		WalletNo:  wallet.WalletNo,
		AssetList: assets,
	}, nil
}

func (s *walletService) TransferOut(ctx context.Context, req models.TransferOutRequest) (*models.TransferOutResponse, error) {
	network := strings.ToLower(strings.TrimSpace(req.Network))
	if network != s.networkCode() {
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	tokenAddress := normalizeTokenAddress(req.TokenAddress)
	if !validateSolanaAddress(req.ToAddress) {
		return nil, newAppError(models.CodeAddressInvalid, "invalid solana address")
	}

	if existing, err := s.store.GetTransactionByRequestNo(ctx, req.RequestNo); err == nil {
		return &models.TransferOutResponse{
			TransactionNo: existing.TransactionNo,
			RequestNo:     existing.RequestNo,
		}, nil
	} else if err != nil && !store.IsNotFound(err) {
		return nil, wrapSystemError(err)
	}

	wallet, err := s.getWallet(ctx, req.WalletNo)
	if err != nil {
		return nil, err
	}
	if !wallet.TransferEnabled {
		return nil, newAppError(models.CodePermissionDenied, "wallet transfer is disabled")
	}

	if s.cfg == nil || s.cfg.Solana == nil || s.cfg.Solana.RPCEndpoint == "" {
		return nil, newAppError(models.CodeSystemBusy, "solana rpc endpoint is not configured")
	}
	blockhash, err := fetchLatestBlockhash(ctx, s.httpClient, s.cfg.Solana.RPCEndpoint)
	if err != nil {
		return nil, wrapSystemError(err)
	}

	var signResult *kmsSignResponse
	tokenSymbol := s.nativeTokenSymbol()
	requestAmount := normalizeAmountString(req.Amount)
	if tokenAddress == models.TokenNative {
		if _, err := amountToLamports(req.Amount); err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		var balanceResp struct {
			Balance float64 `json:"balance"`
		}
		if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/address-balance", map[string]string{
			"address": wallet.Address,
		}, &balanceResp); err != nil {
			return nil, wrapSystemError(err)
		}
		requestAmountValue, _ := parseAmount(req.Amount)
		if balanceResp.Balance < requestAmountValue {
			return nil, newAppError(models.CodeInsufficient, "insufficient balance")
		}
		lamports, _ := amountToLamports(req.Amount)
		unsignedTx, err := buildUnsignedNativeTransferTx(wallet.Address, req.ToAddress, blockhash, lamports, s.computeUnitPrice())
		if err != nil {
			return nil, wrapSystemError(err)
		}
		signResult, err = s.signSolanaTransaction(ctx, wallet, unsignedTx)
		if err != nil {
			return nil, err
		}
	} else {
		tokenMeta, err := s.findConnectorTokenByMint(ctx, tokenAddress)
		if err != nil {
			return nil, err
		}
		var tokenBalanceResp struct {
			Value float64 `json:"value"`
		}
		if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/token-balance", map[string]string{
			"address":   wallet.Address,
			"tokenCode": tokenMeta.Code,
		}, &tokenBalanceResp); err != nil {
			return nil, wrapSystemError(err)
		}
		requestAmountValue, err := parseAmount(req.Amount)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		if tokenBalanceResp.Value < requestAmountValue {
			return nil, newAppError(models.CodeInsufficient, "insufficient balance")
		}
		sourceAccounts, err := fetchTokenAccountsByOwner(ctx, s.httpClient, s.cfg.Solana.RPCEndpoint, wallet.Address, tokenMeta.MintAddress)
		if err != nil {
			return nil, wrapSystemError(err)
		}
		if len(sourceAccounts) == 0 {
			return nil, newAppError(models.CodeInsufficient, "source token account not found")
		}
		destAccounts, err := fetchTokenAccountsByOwner(ctx, s.httpClient, s.cfg.Solana.RPCEndpoint, req.ToAddress, tokenMeta.MintAddress)
		if err != nil {
			return nil, wrapSystemError(err)
		}
		sourceTokenAccount := sourceAccounts[0].Pubkey
		destinationTokenAccount := ""
		createATA := false
		if len(destAccounts) > 0 {
			destinationTokenAccount = destAccounts[0].Pubkey
		} else {
			destinationTokenAccount, createATA, err = deriveAssociatedTokenAddress(req.ToAddress, tokenMeta.MintAddress)
			if err != nil {
				return nil, newAppError(models.CodeAddressInvalid, "invalid destination address")
			}
		}
		baseUnits, err := amountToTokenUnits(req.Amount, tokenMeta.Decimals)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		unsignedTx, err := buildUnsignedSPLTransferTx(wallet.Address, req.ToAddress, tokenMeta.MintAddress, sourceTokenAccount, destinationTokenAccount, blockhash, baseUnits, tokenMeta.Decimals, s.computeUnitPrice(), createATA)
		if err != nil {
			return nil, wrapSystemError(err)
		}
		signResult, err = s.signSolanaTransaction(ctx, wallet, unsignedTx)
		if err != nil {
			return nil, err
		}
		tokenSymbol = tokenMeta.Code
		requestAmount = normalizeAmountString(req.Amount)
	}

	var sendResp struct {
		TxCode string `json:"txCode"`
	}
	if err := s.connectorPost(ctx, "/api/v1/inner/chain-invoke/solana/common/tx-send", map[string]string{
		"txSignResult": signResult.Signature,
	}, &sendResp); err != nil {
		return nil, wrapSystemError(err)
	}

	tx := &models.TransactionEntity{
		TransactionNo: generateID("T"),
		RequestNo:     req.RequestNo,
		Direction:     models.DirectionOut,
		WalletNo:      wallet.WalletNo,
		Network:       wallet.Network,
		FromAddress:   wallet.Address,
		ToAddress:     req.ToAddress,
		TokenAddress:  tokenAddress,
		TokenSymbol:   tokenSymbol,
		Amount:        requestAmount,
		TxHash:        sendResp.TxCode,
		Status:        models.StatusProcessing,
	}
	if err := s.store.CreateTransaction(ctx, tx); err != nil {
		return nil, wrapSystemError(err)
	}

	go s.notifyTransferOutResult(context.Background(), tx)

	return &models.TransferOutResponse{
		TransactionNo: tx.TransactionNo,
		RequestNo:     tx.RequestNo,
	}, nil
}

func (s *walletService) QueryTransaction(ctx context.Context, req models.TransactionQueryRequest) (*models.TransactionQueryResponse, error) {
	tx, err := s.store.GetTransactionByNo(ctx, req.TransactionNo)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, newAppError(models.CodeTxNotFound, "transaction not found")
		}
		return nil, wrapSystemError(err)
	}
	return toTransactionQueryResponse(tx), nil
}

func (s *walletService) QueryHistory(ctx context.Context, req models.TransactionHistoryQueryRequest) (*models.TransactionHistoryQueryResponse, error) {
	if req.PageSize <= 0 || req.PageSize > 100 {
		return nil, newAppError(models.CodeParamError, "pageSize must be between 1 and 100")
	}
	if req.StartTime > 0 && req.EndTime > 0 && req.StartTime > req.EndTime {
		return nil, newAppError(models.CodeTimeRangeInvalid, "startTime must be less than or equal to endTime")
	}
	if req.Direction != "" && req.Direction != models.DirectionIn && req.Direction != models.DirectionOut {
		return nil, newAppError(models.CodeParamError, "direction must be IN or OUT")
	}
	if _, err := s.getWallet(ctx, req.WalletNo); err != nil {
		return nil, err
	}

	rows, err := s.store.QueryHistory(ctx, req)
	if err != nil {
		return nil, wrapSystemError(err)
	}
	items := make([]models.TransactionHistoryItem, 0, len(rows))
	nextCursor := int64(0)
	for _, row := range rows {
		items = append(items, models.TransactionHistoryItem{
			TransactionNo: row.TransactionNo,
			Direction:     row.Direction,
			WalletNo:      row.WalletNo,
			Network:       row.Network,
			FromAddress:   row.FromAddress,
			ToAddress:     row.ToAddress,
			TokenAddress:  row.TokenAddress,
			TokenSymbol:   row.TokenSymbol,
			Amount:        row.Amount,
			Fee:           row.Fee,
			TxHash:        row.TxHash,
			Status:        row.Status,
			TxTime:        row.TxTime,
			CreatedTime:   row.CreatedAt.UnixMilli(),
		})
		nextCursor = row.CreatedAt.UnixMilli()
	}
	if len(rows) < req.PageSize {
		nextCursor = 0
	}
	return &models.TransactionHistoryQueryResponse{Items: items, NextCursor: nextCursor}, nil
}

func (s *walletService) getWallet(ctx context.Context, walletNo string) (*models.WalletEntity, error) {
	wallet, err := s.store.GetWalletByNo(ctx, walletNo)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, newAppError(models.CodeWalletNotFound, "wallet not found")
		}
		return nil, wrapSystemError(err)
	}
	if strings.ToUpper(wallet.Status) != "ACTIVE" {
		return nil, newAppError(models.CodeStatusInvalid, "wallet status is not active")
	}
	if wallet.Network != s.networkCode() {
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	return wallet, nil
}

type connectorToken struct {
	Code        string `json:"code"`
	NetworkCode string `json:"networkCode"`
	MintAddress string `json:"mintAddress"`
	Decimals    uint8  `json:"decimals"`
}

func (s *walletService) listConnectorTokens(ctx context.Context) ([]connectorToken, error) {
	var resp struct {
		Tokens []connectorToken `json:"tokens"`
	}
	if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/token-list", map[string]string{
		"networkCode": s.networkCode(),
	}, &resp); err != nil {
		return nil, wrapSystemError(err)
	}
	return resp.Tokens, nil
}

func (s *walletService) getConnectorToken(ctx context.Context, tokenCode string) (*connectorToken, error) {
	var resp connectorToken
	if err := s.connectorPost(ctx, "/api/v1/inner/chain-data/solana/common/token-get", map[string]string{
		"code": tokenCode,
	}, &resp); err != nil {
		return nil, wrapSystemError(err)
	}
	return &resp, nil
}

func (s *walletService) findConnectorTokenByMint(ctx context.Context, mintAddress string) (*connectorToken, error) {
	tokens, err := s.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if strings.EqualFold(token.MintAddress, mintAddress) {
			tokenCopy := token
			return &tokenCopy, nil
		}
	}
	return nil, newAppError(models.CodeTokenUnsupported, "token not supported")
}

type kmsSignResponse struct {
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

func (s *walletService) signSolanaTransaction(ctx context.Context, wallet *models.WalletEntity, unsignedTx string) (*kmsSignResponse, error) {
	body := map[string]string{
		"keystore_id": wallet.KMSKeystoreID,
		"password":    wallet.KMSPassword,
		"tx_json":     unsignedTx,
	}
	var data kmsSignResponse
	var path string
	switch strings.ToLower(wallet.KMSKeyType) {
	case "privatekey", "solana":
		path = "/kms/privatekey/sign-tx/solana"
	case "mnemonic":
		path = fmt.Sprintf("/kms/mnemonic/sign-tx/solana/%s/%s/%s", defaultIndex(wallet.AccountIndex), defaultIndex(wallet.ChangeIndex), defaultIndex(wallet.AddressIndex))
	default:
		return nil, newAppError(models.CodeSystemBusy, "unsupported kms key type")
	}
	if err := s.kmsPost(ctx, path, body, &data); err != nil {
		return nil, wrapSystemError(err)
	}
	return &data, nil
}

func (s *walletService) connectorPost(ctx context.Context, path string, reqBody interface{}, data interface{}) error {
	baseURL := ""
	username := ""
	password := ""
	if s.cfg != nil && s.cfg.Connector != nil {
		baseURL = s.cfg.Connector.BaseURL
		username = s.cfg.Connector.Username
		password = s.cfg.Connector.Password
	}
	return s.doJSONRequest(ctx, baseURL, path, username, password, reqBody, data)
}

func (s *walletService) kmsPost(ctx context.Context, path string, reqBody interface{}, data interface{}) error {
	baseURL := ""
	username := ""
	password := ""
	if s.cfg != nil && s.cfg.KMS != nil {
		baseURL = s.cfg.KMS.BaseURL
		username = s.cfg.KMS.Username
		password = s.cfg.KMS.Password
	}
	return s.doJSONRequest(ctx, baseURL, path, username, password, reqBody, data)
}

func (s *walletService) doJSONRequest(ctx context.Context, baseURL string, path string, username string, password string, reqBody interface{}, data interface{}) error {
	if baseURL == "" {
		return fmt.Errorf("baseURL is empty")
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status=%d body=%s", resp.StatusCode, string(raw))
	}

	var envelope struct {
		Code    interface{}     `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if normalizeRespCode(envelope.Code) != models.CodeSuccess && normalizeRespCode(envelope.Code) != "200" {
		if envelope.Message == "" {
			envelope.Message = string(raw)
		}
		return fmt.Errorf(envelope.Message)
	}
	if data == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	return json.Unmarshal(envelope.Data, data)
}

func (s *walletService) notifyDeposit(ctx context.Context, tx *models.TransactionEntity) {
	payload := models.DepositNotifyRequest{
		NotifyID:      generateID("N"),
		TransactionNo: tx.TransactionNo,
		WalletNo:      tx.WalletNo,
		Network:       tx.Network,
		Address:       tx.ToAddress,
		FromAddress:   tx.FromAddress,
		TokenAddress:  tx.TokenAddress,
		TokenSymbol:   tx.TokenSymbol,
		Amount:        tx.Amount,
		TxHash:        tx.TxHash,
		Status:        tx.Status,
		TxTime:        tx.TxTime,
		NotifyTime:    time.Now().UnixMilli(),
	}
	if tx.TokenAddress == models.TokenNative {
		payload.TokenAddress = ""
	}
	if s.cfg == nil || s.cfg.Callback == nil || s.cfg.Callback.DepositURL == "" {
		log.Infof("skip deposit notify callback: depositUrl is empty payload=%+v", payload)
		return
	}
	log.Infof("deposit notify request url=%s payload=%+v", s.cfg.Callback.DepositURL, payload)
	if err := s.postSignedCallback(ctx, s.cfg.Callback.DepositURL, payload); err != nil {
		log.Warningf("deposit notify callback failed url=%s err=%v payload=%+v", s.cfg.Callback.DepositURL, err, payload)
		return
	}
	log.Infof("deposit notify callback success url=%s transactionNo=%s", s.cfg.Callback.DepositURL, tx.TransactionNo)
}

func (s *walletService) notifyTransferOutResult(ctx context.Context, tx *models.TransactionEntity) {
	payload := models.TransferOutNotifyRequest{
		NotifyID:      generateID("N"),
		TransactionNo: tx.TransactionNo,
		RequestNo:     tx.RequestNo,
		WalletNo:      tx.WalletNo,
		Network:       tx.Network,
		ToAddress:     tx.ToAddress,
		TokenAddress:  tx.TokenAddress,
		TokenSymbol:   tx.TokenSymbol,
		Amount:        tx.Amount,
		Fee:           tx.Fee,
		TxHash:        tx.TxHash,
		Status:        tx.Status,
		FailReason:    tx.FailReason,
		TxTime:        tx.TxTime,
		NotifyTime:    time.Now().UnixMilli(),
	}
	if tx.TokenAddress == models.TokenNative {
		payload.TokenAddress = ""
	}
	if s.cfg == nil || s.cfg.Callback == nil || s.cfg.Callback.TransferOutURL == "" {
		log.Infof("skip transfer out notify callback: transferOutUrl is empty payload=%+v", payload)
		return
	}
	log.Infof("transfer out notify request url=%s payload=%+v", s.cfg.Callback.TransferOutURL, payload)
	if err := s.postSignedCallback(ctx, s.cfg.Callback.TransferOutURL, payload); err != nil {
		log.Warningf("transfer out notify callback failed url=%s err=%v payload=%+v", s.cfg.Callback.TransferOutURL, err, payload)
		return
	}
	log.Infof("transfer out notify callback success url=%s transactionNo=%s", s.cfg.Callback.TransferOutURL, tx.TransactionNo)
}

func (s *walletService) postSignedCallback(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	appID := ""
	secret := ""
	if s.cfg != nil && s.cfg.Signature != nil {
		appID = s.cfg.Signature.AppID
		secret = s.cfg.Signature.AppSecret
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.Header.Set("X-App-Id", appID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signPayload(secret, appID, timestamp, body))
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Warningf("callback request failed url=%s err=%v", url, err)
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		log.Warningf("callback non-200 url=%s status=%d body=%s", url, resp.StatusCode, string(raw))
		return fmt.Errorf("callback status=%d", resp.StatusCode)
	}
	var callbackResp models.CallbackResponse
	if err := json.Unmarshal(raw, &callbackResp); err != nil {
		return err
	}
	if normalizeRespCode(callbackResp.Code) != models.CodeSuccess {
		return fmt.Errorf("callback business code=%v message=%s", callbackResp.Code, callbackResp.Message)
	}
	return nil
}

func signPayload(secret string, appID string, timestamp string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(appID + timestamp + string(body)))
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeRespCode(code interface{}) string {
	switch v := code.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

func toTransactionQueryResponse(tx *models.TransactionEntity) *models.TransactionQueryResponse {
	return &models.TransactionQueryResponse{
		TransactionNo: tx.TransactionNo,
		Direction:     tx.Direction,
		WalletNo:      tx.WalletNo,
		Network:       tx.Network,
		FromAddress:   tx.FromAddress,
		ToAddress:     tx.ToAddress,
		TokenAddress:  tx.TokenAddress,
		TokenSymbol:   tx.TokenSymbol,
		Amount:        tx.Amount,
		Fee:           tx.Fee,
		TxHash:        tx.TxHash,
		Status:        tx.Status,
		FailReason:    tx.FailReason,
		TxTime:        tx.TxTime,
		CreatedTime:   tx.CreatedAt.UnixMilli(),
		UpdatedTime:   tx.UpdatedAt.UnixMilli(),
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func parseAmount(v string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount")
	}
	return value, nil
}

func normalizeAmountString(v string) string {
	value, err := parseAmount(v)
	if err != nil {
		return v
	}
	return formatFloat(value)
}

func normalizeTokenAddress(v string) string {
	if strings.TrimSpace(v) == "" {
		return models.TokenNative
	}
	if strings.EqualFold(strings.TrimSpace(v), models.TokenNative) {
		return models.TokenNative
	}
	return strings.TrimSpace(v)
}

func defaultIndex(v string) string {
	if strings.TrimSpace(v) == "" {
		return "0"
	}
	return strings.TrimSpace(v)
}

func (s *walletService) networkCode() string {
	if s.cfg != nil && s.cfg.Connector != nil && s.cfg.Connector.NetworkCode != "" {
		return strings.ToLower(s.cfg.Connector.NetworkCode)
	}
	return models.NetworkSolana
}

func (s *walletService) nativeTokenSymbol() string {
	if s.cfg != nil && s.cfg.Connector != nil && s.cfg.Connector.NativeTokenSymbol != "" {
		return s.cfg.Connector.NativeTokenSymbol
	}
	return "SOL"
}

func (s *walletService) computeUnitPrice() uint64 {
	if s.cfg != nil && s.cfg.Solana != nil {
		return s.cfg.Solana.ComputeUnitPrice
	}
	return 0
}
