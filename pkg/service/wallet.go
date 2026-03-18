package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/signature"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
)

type WalletService interface {
	SyncSubscriptions() error
	EnsureWalletNetworks() error
	CreateWallet(ctx context.Context, req models.WalletCreateRequest) (*models.WalletCreateResponse, error)
	QueryWalletInfo(ctx context.Context, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error)
	QueryTransferOutAssets(ctx context.Context, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error)
	TransferOut(ctx context.Context, req models.TransferOutRequest) (*models.TransferOutResponse, error)
	QueryTransaction(ctx context.Context, req models.TransactionQueryRequest) (*models.TransactionQueryResponse, error)
	QueryHistory(ctx context.Context, req models.TransactionHistoryQueryRequest) (*models.TransactionHistoryQueryResponse, error)
	HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error
	HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error
}

func NewWalletService(cfg *config.Config, st store.Store, httpClient *http.Client) WalletService {
	svc := &walletService{
		cfg:        cfg,
		store:      st,
		httpClient: httpClient,
	}
	svc.providers = buildNetworkProviders(svc)
	return svc
}

type walletService struct {
	cfg        *config.Config
	store      store.Store
	httpClient *http.Client
	providers  map[string]networkWalletProvider
}

func (s *walletService) SyncSubscriptions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, provider := range s.providers {
		if err := provider.SyncSubscriptions(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *walletService) CreateWallet(ctx context.Context, req models.WalletCreateRequest) (*models.WalletCreateResponse, error) {
	return s.createWallets(ctx, "", "")
}

func (s *walletService) EnsureWalletNetworks() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	wallets, err := s.store.ListActiveWallets(ctx, "")
	if err != nil {
		return wrapSystemError(err)
	}
	seen := make(map[string]struct{}, len(wallets))
	for _, wallet := range wallets {
		if strings.TrimSpace(wallet.WalletNo) == "" {
			continue
		}
		if _, ok := seen[wallet.WalletNo]; ok {
			continue
		}
		seen[wallet.WalletNo] = struct{}{}
		if _, err := s.createWallets(ctx, wallet.WalletNo, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *walletService) createWallets(ctx context.Context, masterWalletNo string, requestedNetwork string) (*models.WalletCreateResponse, error) {
	networks, err := s.resolveCreateNetworks(requestedNetwork)
	if err != nil {
		return nil, err
	}
	masterWalletNo = strings.TrimSpace(masterWalletNo)

	items := make([]models.WalletCreateItem, 0, len(networks))
	if masterWalletNo != "" {
		existingWallets, err := s.getWallets(ctx, masterWalletNo)
		if err != nil {
			appErr, ok := err.(*AppError)
			if !ok || appErr.Code != models.CodeWalletNotFound {
				return nil, err
			}
		} else {
			existingByNetwork := make(map[string]models.WalletCreateItem, len(existingWallets))
			for _, wallet := range existingWallets {
				existingByNetwork[normalizedNetwork(wallet.Network)] = models.WalletCreateItem{
					Network: wallet.Network,
					Address: wallet.Address,
				}
			}
			filtered := make([]string, 0, len(networks))
			for _, network := range networks {
				if item, ok := existingByNetwork[network]; ok {
					items = append(items, item)
					continue
				}
				filtered = append(filtered, network)
			}
			networks = filtered
		}
	}

	for _, network := range networks {
		provider, err := s.provider(network)
		if err != nil {
			return nil, err
		}
		resp, err := provider.CreateWallet(ctx, walletCreateOptions{
			WalletNo: masterWalletNo,
			Network:  network,
		})
		if err != nil {
			return nil, err
		}
		if masterWalletNo == "" {
			masterWalletNo = resp.WalletNo
		}
		if len(resp.Wallets) > 0 {
			items = append(items, resp.Wallets[0])
			continue
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Network < items[j].Network
	})
	response := &models.WalletCreateResponse{
		WalletNo: masterWalletNo,
		Wallets:  items,
	}
	return response, nil
}

func (s *walletService) QueryWalletInfo(ctx context.Context, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error) {
	log.Infof("wallet info query start walletNo=%s network=%s", req.WalletNo, req.Network)
	if normalizedNetwork(req.Network) != "" {
		wallet, err := s.getWallet(ctx, req.WalletNo, req.Network)
		if err != nil {
			log.Warningf("wallet info query failed to resolve wallet walletNo=%s network=%s err=%v", req.WalletNo, req.Network, err)
			return nil, err
		}
		provider, _ := s.provider(wallet.Network)
		log.Infof("wallet info query dispatch single walletNo=%s network=%s address=%s", wallet.WalletNo, wallet.Network, wallet.Address)
		resp, err := provider.QueryWalletInfo(ctx, wallet, req)
		if err != nil {
			log.Warningf("wallet info query failed walletNo=%s network=%s err=%v", wallet.WalletNo, wallet.Network, err)
			return nil, err
		}
		log.Infof("wallet info query success walletNo=%s network=%s tokenCount=%d", wallet.WalletNo, wallet.Network, len(resp.Tokens))
		return resp, nil
	}

	wallets, err := s.getWallets(ctx, req.WalletNo)
	if err != nil {
		log.Warningf("wallet info query failed to list wallets walletNo=%s err=%v", req.WalletNo, err)
		return nil, err
	}
	log.Infof("wallet info query aggregate walletNo=%s walletCount=%d", req.WalletNo, len(wallets))
	balances := make(map[string]*big.Rat)
	for _, wallet := range wallets {
		provider, err := s.provider(wallet.Network)
		if err != nil {
			log.Warningf("wallet info query failed to load provider walletNo=%s network=%s err=%v", wallet.WalletNo, wallet.Network, err)
			return nil, err
		}
		log.Infof("wallet info query dispatch aggregate walletNo=%s network=%s address=%s", wallet.WalletNo, wallet.Network, wallet.Address)
		resp, err := provider.QueryWalletInfo(ctx, &wallet, models.WalletInfoQueryRequest{
			WalletNo: req.WalletNo,
			Network:  wallet.Network,
		})
		if err != nil {
			log.Warningf("wallet info query aggregate failed walletNo=%s network=%s err=%v", wallet.WalletNo, wallet.Network, err)
			return nil, err
		}
		log.Infof("wallet info query aggregate partial success walletNo=%s network=%s tokenCount=%d", wallet.WalletNo, wallet.Network, len(resp.Tokens))
		for _, token := range resp.Tokens {
			if _, ok := balances[token.TokenSymbol]; !ok {
				balances[token.TokenSymbol] = new(big.Rat)
			}
			if err := addDecimalString(balances[token.TokenSymbol], token.Balance); err != nil {
				log.Warningf("wallet info query aggregate parse balance failed walletNo=%s network=%s token=%s balance=%s err=%v", wallet.WalletNo, wallet.Network, token.TokenSymbol, token.Balance, err)
				return nil, newAppError(models.CodeSystemBusy, err.Error())
			}
		}
	}

	tokenSymbols := make([]string, 0, len(balances))
	for symbol := range balances {
		tokenSymbols = append(tokenSymbols, symbol)
	}
	sort.Strings(tokenSymbols)

	items := make([]models.WalletTokenBalance, 0, len(tokenSymbols))
	for _, symbol := range tokenSymbols {
		items = append(items, models.WalletTokenBalance{
			TokenSymbol: symbol,
			Balance:     balances[symbol].FloatString(6),
		})
	}
	resp := &models.WalletInfoQueryResponse{
		WalletNo: req.WalletNo,
		Tokens:   items,
	}
	log.Infof("wallet info query aggregate success walletNo=%s tokenCount=%d", req.WalletNo, len(resp.Tokens))
	return resp, nil
}

func (s *walletService) QueryTransferOutAssets(ctx context.Context, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error) {
	wallet, err := s.getWallet(ctx, req.WalletNo, req.Network)
	if err != nil {
		return nil, err
	}
	provider, _ := s.provider(wallet.Network)
	return provider.QueryTransferOutAssets(ctx, wallet, req)
}

func (s *walletService) TransferOut(ctx context.Context, req models.TransferOutRequest) (*models.TransferOutResponse, error) {
	wallet, err := s.getWallet(ctx, req.WalletNo, req.Network)
	if err != nil {
		return nil, err
	}
	if req.Network != "" && normalizedNetwork(req.Network) != normalizedNetwork(wallet.Network) {
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	provider, _ := s.provider(wallet.Network)
	req.Network = wallet.Network
	return provider.TransferOut(ctx, wallet, req)
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
	if _, err := s.getWallets(ctx, req.WalletNo); err != nil {
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

type connectorToken struct {
	TokenCode    string `json:"tokenCode"`
	NetworkCode  string `json:"networkCode"`
	TokenAddress string `json:"tokenAddress"`
	Decimals     uint8  `json:"decimals"`
}

type kmsSignResponse struct {
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}

type SignEIP7702Request struct {
	KeystoreId string `json:"keystore_id,omitempty"`
	Password   string `json:"password,omitempty"`
	ChainID    string `json:"chainId,omitempty"`
	Address    string `json:"address,omitempty"`
	Nonce      uint64 `json:"nonce,omitempty"`
}

type SignedEIP7702Res struct {
	Hash      string `json:"hash,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type SignTypedDataRequest struct {
	KeystoreId string `json:"keystore_id,omitempty"`
	Password   string `json:"password,omitempty"`
	TypedData  string `json:"typed_data,omitempty"`
}

type SignDataRes struct {
	Hash      string `json:"hash,omitempty"`
	Signature string `json:"signature,omitempty"`
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

func (s *walletService) signEVMTransaction(ctx context.Context, wallet *models.WalletEntity, unsignedTx string) (*kmsSignResponse, error) {
	if strings.TrimSpace(unsignedTx) == "" {
		return nil, newAppError(models.CodeSystemBusy, "empty unsigned evm transaction")
	}
	body := map[string]string{
		"keystore_id": wallet.KMSKeystoreID,
		"password":    wallet.KMSPassword,
		"tx_json":     unsignedTx,
	}
	var data kmsSignResponse
	var path string
	switch strings.ToLower(wallet.KMSKeyType) {
	case "privatekey", "evm", "ethereum":
		path = "/kms/privatekey/sign-tx/evm"
	case "mnemonic":
		path = fmt.Sprintf("/kms/mnemonic/sign-tx/evm/%s/%s/%s", defaultIndex(wallet.AccountIndex), defaultIndex(wallet.ChangeIndex), defaultIndex(wallet.AddressIndex))
	default:
		return nil, newAppError(models.CodeSystemBusy, "unsupported kms key type")
	}
	if err := s.kmsPost(ctx, path, body, &data); err != nil {
		return nil, wrapSystemError(err)
	}
	return &data, nil
}

func (s *walletService) signEIP7702Authorization(ctx context.Context, wallet *models.WalletEntity, chainID uint64, address string, nonce uint64) (*SignedEIP7702Res, error) {
	if strings.ToLower(wallet.KMSKeyType) != "mnemonic" {
		return nil, newAppError(models.CodeSystemBusy, "eip7702 requires mnemonic kms key type")
	}
	body := SignEIP7702Request{
		KeystoreId: wallet.KMSKeystoreID,
		Password:   wallet.KMSPassword,
		ChainID:    strconv.FormatUint(chainID, 10),
		Address:    address,
		Nonce:      nonce,
	}
	var data SignedEIP7702Res
	path := fmt.Sprintf("/kms/mnemonic/sign-eip7702/evm/%s/%s/%s", defaultIndex(wallet.AccountIndex), defaultIndex(wallet.ChangeIndex), defaultIndex(wallet.AddressIndex))
	if err := s.kmsPost(ctx, path, body, &data); err != nil {
		return nil, wrapSystemError(err)
	}
	return &data, nil
}

func (s *walletService) signEIP712TypedData(ctx context.Context, wallet *models.WalletEntity, typedData string) (*SignDataRes, error) {
	if strings.TrimSpace(typedData) == "" {
		return nil, newAppError(models.CodeSystemBusy, "empty typed data")
	}
	if strings.ToLower(wallet.KMSKeyType) != "mnemonic" {
		return nil, newAppError(models.CodeSystemBusy, "typed data signing requires mnemonic kms key type")
	}
	body := SignTypedDataRequest{
		KeystoreId: wallet.KMSKeystoreID,
		Password:   wallet.KMSPassword,
		TypedData:  typedData,
	}
	var data SignDataRes
	path := fmt.Sprintf("/kms/mnemonic/sign-typed-data/evm/%s/%s/%s", defaultIndex(wallet.AccountIndex), defaultIndex(wallet.ChangeIndex), defaultIndex(wallet.AddressIndex))
	if err := s.kmsPost(ctx, path, body, &data); err != nil {
		return nil, wrapSystemError(err)
	}
	return &data, nil
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
	requestURL := strings.TrimRight(baseURL, "/") + path
	log.Infof("downstream request start url=%s payload=%s", requestURL, truncateLogString(string(payload), 512))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payload))
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
	log.Infof("downstream request done url=%s status=%d body=%s", requestURL, resp.StatusCode, truncateLogString(string(raw), 512))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("request url=%s http status=%d body=%s", requestURL, resp.StatusCode, string(raw))
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

func truncateLogString(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
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
	if GetApp() == nil || GetApp().SignatureKey == nil {
		return fmt.Errorf("signature service is unavailable")
	}
	publickeyID, signKey, err := GetApp().SignatureKey.DefaultKey()
	if err != nil {
		return err
	}
	if publickeyID == "" || signKey == nil || signKey.PrivateKey == "" {
		return fmt.Errorf("signature key is not configured")
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signatureValue, err := signature.SignBase64(
		signKey.PrivateKey,
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		body,
		timestamp,
	)
	if err != nil {
		return err
	}
	req.Header.Set("X-Publickey-ID", publickeyID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Signature", signatureValue)
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

func normalizeTokenSymbol(v string, nativeSymbol string) string {
	if strings.TrimSpace(v) == "" {
		return strings.ToUpper(strings.TrimSpace(nativeSymbol))
	}
	if strings.EqualFold(strings.TrimSpace(v), models.TokenNative) {
		return strings.ToUpper(strings.TrimSpace(nativeSymbol))
	}
	return strings.ToUpper(strings.TrimSpace(v))
}

func addDecimalString(dst *big.Rat, value string) error {
	if dst == nil {
		return fmt.Errorf("nil destination")
	}
	parsed, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return fmt.Errorf("invalid decimal value: %s", value)
	}
	dst.Add(dst, parsed)
	return nil
}

func defaultIndex(v string) string {
	if strings.TrimSpace(v) == "" {
		return "0"
	}
	return strings.TrimSpace(v)
}
