package service

import (
	"context"
	"encoding/json"
	"math/big"
	"regexp"
	"strings"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
)

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

type evmProvider struct {
	svc     *walletService
	network string
}

type evmToken struct {
	Code            string `json:"code"`
	NetworkCode     string `json:"networkCode"`
	ContractAddress string `json:"contractAddress"`
	MintAddress     string `json:"mintAddress"`
	Decimals        uint8  `json:"decimals"`
}

func (p *evmProvider) NetworkCode() string {
	return p.network
}

func (p *evmProvider) SyncSubscriptions(ctx context.Context) error {
	wallets, err := p.svc.store.ListActiveWallets(ctx, p.network)
	if err != nil {
		return err
	}
	for _, wallet := range wallets {
		if !wallet.DepositEnabled {
			continue
		}
		if err := p.subscribeAddress(ctx, wallet.Address); err != nil {
			return err
		}
	}
	return nil
}

func (p *evmProvider) CreateWallet(ctx context.Context, opts walletCreateOptions) (*models.WalletCreateResponse, error) {
	walletNo := strings.TrimSpace(opts.WalletNo)
	password := ""
	keystoreID := ""
	keyType := "mnemonic"
	accountIndex := "0"
	changeIndex := "0"
	addressIndex := "0"

	if walletNo != "" {
		if existing, err := p.svc.store.GetWalletByNoAndNetwork(ctx, walletNo, p.network); err == nil {
			return newWalletCreateResponse(existing.WalletNo, models.WalletCreateItem{
				Network: existing.Network,
				Address: existing.Address,
			}), nil
		} else if err != nil && !store.IsNotFound(err) {
			return nil, wrapSystemError(err)
		}
	}

	if walletNo != "" {
		existingWallets, err := p.svc.getWallets(ctx, walletNo)
		if err != nil {
			appErr, ok := err.(*AppError)
			if !ok || appErr.Code != models.CodeWalletNotFound {
				return nil, err
			}
		} else if len(existingWallets) > 0 {
			shared := existingWallets[0]
			keystoreID = shared.KMSKeystoreID
			password = shared.KMSPassword
			keyType = shared.KMSKeyType
			accountIndex = shared.AccountIndex
			changeIndex = shared.ChangeIndex
			addressIndex = shared.AddressIndex
		}
	}

	if walletNo == "" {
		walletNo = generateWalletNo()
	}
	if keystoreID == "" {
		password = generatePassword()
		var keyStoreResp struct {
			KeystoreId string `json:"keystore_id"`
		}
		if err := p.svc.kmsPost(ctx, "/kms/keystore/create", map[string]string{
			"password": password,
		}, &keyStoreResp); err != nil {
			return nil, wrapSystemError(err)
		}
		keystoreID = keyStoreResp.KeystoreId
	}

	var addressResp []struct {
		Network string `json:"chain_type"`
		Address string `json:"address"`
	}
	if err := p.svc.kmsPost(ctx, "/kms/keystore/address/0/0/0", map[string]string{
		"keystore_id": keystoreID,
		"password":    password,
	}, &addressResp); err != nil {
		var wrappedAddressResp struct {
			KeyAddress []struct {
				Network string `json:"chain_type"`
				Address string `json:"address"`
			} `json:"key_address"`
		}
		if err = p.svc.kmsPost(ctx, "/kms/keystore/address/0/0/0", map[string]string{
			"keystore_id": keystoreID,
			"password":    password,
		}, &wrappedAddressResp); err != nil {
			return nil, wrapSystemError(err)
		}
		addressResp = wrappedAddressResp.KeyAddress
	}

	address := ""
	for _, item := range addressResp {
		if p.isEVMAddressType(item.Network) && validateEVMAddress(item.Address) {
			address = item.Address
			break
		}
	}
	if address == "" {
		return nil, newAppError(models.CodeSystemBusy, "failed to derive wallet address")
	}

	wallet := &models.WalletEntity{
		WalletNo:        walletNo,
		Network:         p.network,
		Address:         address,
		KMSKeystoreID:   keystoreID,
		KMSPassword:     password,
		KMSKeyType:      keyType,
		AccountIndex:    accountIndex,
		ChangeIndex:     changeIndex,
		AddressIndex:    addressIndex,
		DepositEnabled:  true,
		TransferEnabled: true,
		Status:          "ACTIVE",
	}
	if err := p.svc.store.CreateWallet(ctx, wallet); err != nil {
		return nil, wrapSystemError(err)
	}
	if err := p.subscribeAddress(ctx, address); err != nil {
		return nil, wrapSystemError(err)
	}

	return newWalletCreateResponse(wallet.WalletNo, models.WalletCreateItem{
		Network: wallet.Network,
		Address: wallet.Address,
	}), nil
}

func (p *evmProvider) QueryWalletInfo(ctx context.Context, wallet *models.WalletEntity, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error) {
	items := make([]models.WalletTokenBalance, 0, 8)
	var nativeResp struct {
		Balance float64 `json:"balance"`
	}
	if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/address-balance", map[string]string{
		"address": wallet.Address,
	}, &nativeResp); err != nil {
		return nil, wrapSystemError(err)
	}
	items = append(items, models.WalletTokenBalance{
		TokenSymbol: p.svc.nativeTokenSymbol(p.network),
		Balance:     formatFloat(nativeResp.Balance),
	})

	tokens, err := p.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		var tokenResp struct {
			Value float64 `json:"value"`
		}
		if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/token-balance", map[string]string{
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

func (p *evmProvider) QueryTransferOutAssets(ctx context.Context, wallet *models.WalletEntity, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error) {
	if !wallet.TransferEnabled {
		return nil, newAppError(models.CodePermissionDenied, "wallet transfer is disabled")
	}
	assets := []models.TransferableAsset{{
		Network:      wallet.Network,
		TokenAddress: models.TokenNative,
		TokenSymbol:  p.svc.nativeTokenSymbol(p.network),
	}}
	tokens, err := p.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		assets = append(assets, models.TransferableAsset{
			Network:      wallet.Network,
			TokenAddress: token.Address(),
			TokenSymbol:  token.Code,
		})
	}
	return &models.TransferOutQueryResponse{
		WalletNo:  wallet.WalletNo,
		AssetList: assets,
	}, nil
}

func (p *evmProvider) TransferOut(ctx context.Context, wallet *models.WalletEntity, req models.TransferOutRequest) (*models.TransferOutResponse, error) {
	tokenSymbolInput := normalizeTokenSymbol(req.TokenSymbol, p.svc.nativeTokenSymbol(p.network))
	tokenAddress := models.TokenNative
	if !validateEVMAddress(req.ToAddress) {
		return nil, newAppError(models.CodeAddressInvalid, "invalid evm address")
	}
	if existing, err := p.svc.store.GetTransactionByRequestNo(ctx, req.RequestNo); err == nil {
		return &models.TransferOutResponse{
			TransactionNo: existing.TransactionNo,
			RequestNo:     existing.RequestNo,
		}, nil
	} else if err != nil && !store.IsNotFound(err) {
		return nil, wrapSystemError(err)
	}
	if !wallet.TransferEnabled {
		return nil, newAppError(models.CodePermissionDenied, "wallet transfer is disabled")
	}
	requestAmount := normalizeAmountString(req.Amount)
	if p.eip7702Enabled() {
		return p.transferOutWithEIP7702(ctx, wallet, req, tokenSymbolInput, tokenAddress, requestAmount)
	}
	rpcClient, chainID, gasPrice, nonce, err := p.prepareTransferBuild(ctx, wallet.Address)
	if err != nil {
		return nil, err
	}
	_ = rpcClient

	var signResult *kmsSignResponse
	tokenSymbol := p.svc.nativeTokenSymbol(p.network)
	if strings.EqualFold(tokenSymbolInput, tokenSymbol) {
		var balanceResp struct {
			Balance float64 `json:"balance"`
		}
		if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/address-balance", map[string]string{
			"address": wallet.Address,
		}, &balanceResp); err != nil {
			return nil, wrapSystemError(err)
		}
		requestAmountValue, err := parseAmount(req.Amount)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		if balanceResp.Balance < requestAmountValue {
			return nil, newAppError(models.CodeInsufficient, "insufficient balance")
		}

		gasLimit, err := p.estimateNativeTransferGas(ctx, rpcClient, wallet.Address, req.ToAddress, gasPrice, requestAmount)
		if err != nil {
			return nil, err
		}
		unsignedTx, err := buildUnsignedEVMNativeTransferTx(wallet.Address, req.ToAddress, chainID, nonce, gasPrice, gasLimit, requestAmount)
		if err != nil {
			return nil, err
		}
		signResult, err = p.svc.signEVMTransaction(ctx, wallet, unsignedTx)
		if err != nil {
			return nil, err
		}
	} else {
		tokenMeta, err := p.findConnectorTokenByCode(ctx, tokenSymbolInput)
		if err != nil {
			return nil, err
		}
		tokenAddress = tokenMeta.Address()
		var tokenBalanceResp struct {
			Value float64 `json:"value"`
		}
		if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/token-balance", map[string]string{
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

		gasLimit, err := p.estimateERC20TransferGas(ctx, rpcClient, wallet.Address, tokenMeta.Address(), req.ToAddress, gasPrice, requestAmount, tokenMeta.Decimals)
		if err != nil {
			return nil, err
		}
		unsignedTx, err := buildUnsignedERC20TransferTx(tokenMeta.Address(), req.ToAddress, chainID, nonce, gasPrice, gasLimit, requestAmount, tokenMeta.Decimals)
		if err != nil {
			return nil, err
		}
		signResult, err = p.svc.signEVMTransaction(ctx, wallet, unsignedTx)
		if err != nil {
			return nil, err
		}
		tokenSymbol = tokenMeta.Code
	}

	var sendResp struct {
		TxCode string `json:"txCode"`
	}
	if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-invoke/evm/common/tx-send", map[string]string{
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
	if err := p.svc.store.CreateTransaction(ctx, tx); err != nil {
		return nil, wrapSystemError(err)
	}

	go p.svc.notifyTransferOutResult(context.Background(), tx)

	return &models.TransferOutResponse{
		TransactionNo: tx.TransactionNo,
		RequestNo:     tx.RequestNo,
	}, nil
}

func (p *evmProvider) transferOutWithEIP7702(ctx context.Context, wallet *models.WalletEntity, req models.TransferOutRequest, tokenSymbolInput string, tokenAddress string, requestAmount string) (*models.TransferOutResponse, error) {
	connector := p.svc.connectorConfig(p.network)
	if connector == nil {
		return nil, newAppError(models.CodeSystemBusy, "evm connector is not configured")
	}
	delegator := strings.TrimSpace(connector.EIP7702Delegator)
	if !validateEVMAddress(delegator) {
		return nil, newAppError(models.CodeSystemBusy, "invalid eip7702 delegator address")
	}
	entryPoint := strings.TrimSpace(connector.EntryPoint)
	if entryPoint == "" {
		entryPoint = entryPointV08Default
	}
	if !validateEVMAddress(entryPoint) {
		return nil, newAppError(models.CodeSystemBusy, "invalid entry point address")
	}

	rpcClient, chainID, gasPrice, txNonce, err := p.prepareTransferBuild(ctx, wallet.Address)
	if err != nil {
		return nil, err
	}

	bundlerClient, err := p.newBundlerRPCClient()
	if err != nil {
		return nil, err
	}

	callData := "0x"
	callTarget := req.ToAddress
	transferToAddress := req.ToAddress
	tokenSymbol := p.svc.nativeTokenSymbol(p.network)
	callValue := big.NewInt(0)
	callGasLimit := p.userOperationCallGasLimit(true)

	if strings.EqualFold(tokenSymbolInput, tokenSymbol) {
		var balanceResp struct {
			Balance float64 `json:"balance"`
		}
		if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/address-balance", map[string]string{
			"address": wallet.Address,
		}, &balanceResp); err != nil {
			return nil, wrapSystemError(err)
		}
		requestAmountValue, err := parseAmount(req.Amount)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		if balanceResp.Balance < requestAmountValue {
			return nil, newAppError(models.CodeInsufficient, "insufficient balance")
		}
		callValue, err = decimalToBaseUnits(requestAmount, 18)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
	} else {
		tokenMeta, err := p.findConnectorTokenByCode(ctx, tokenSymbolInput)
		if err != nil {
			return nil, err
		}
		tokenAddress = tokenMeta.Address()
		var tokenBalanceResp struct {
			Value float64 `json:"value"`
		}
		if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/token-balance", map[string]string{
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
		amountUnits, err := decimalToBaseUnits(requestAmount, tokenMeta.Decimals)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		callData, err = buildERC20TransferData(transferToAddress, amountUnits)
		if err != nil {
			return nil, newAppError(models.CodeParamError, err.Error())
		}
		tokenSymbol = tokenMeta.Code
		callGasLimit = p.userOperationCallGasLimit(false)
		callTarget = tokenMeta.Address()
	}

	userOpCallData, err := buildEIP7702ExecuteCallData(callTarget, callValue, callData)
	if err != nil {
		return nil, newAppError(models.CodeParamError, err.Error())
	}
	userOpNonce, err := rpcClient.getUserOperationNonce(ctx, entryPoint, wallet.Address)
	if err != nil {
		return nil, wrapSystemError(err)
	}
	userOp := evmUserOperation{
		Sender:               wallet.Address,
		Nonce:                formatHexUint64(userOpNonce),
		Factory:              eip7702FactoryFlag,
		FactoryData:          "0x",
		CallData:             userOpCallData,
		CallGasLimit:         formatHexUint64(callGasLimit),
		VerificationGasLimit: formatHexUint64(0),
		PreVerificationGas:   formatHexUint64(0),
		MaxPriorityFeePerGas: formatHexBig(gasPrice),
		MaxFeePerGas:         formatHexBig(gasPrice),
		Signature:            "0x",
	}
	existingDelegator, alreadyDelegated, err := p.lookupEIP7702Delegator(ctx, rpcClient, wallet.Address)
	if err != nil {
		return nil, err
	}
	if alreadyDelegated && !strings.EqualFold(existingDelegator, delegator) {
		return nil, newAppError(models.CodeSystemBusy, "account is already delegated to another eip7702 contract")
	}
	stateOverride, err := p.userOperationEstimateStateOverride(wallet.Address, delegator, alreadyDelegated)
	if err != nil {
		return nil, err
	}
	gasEstimate, err := bundlerClient.estimateUserOperationGas(ctx, userOp, entryPoint, stateOverride)
	if err != nil {
		return nil, wrapSystemError(err)
	}
	if strings.TrimSpace(gasEstimate.CallGasLimit) != "" {
		userOp.CallGasLimit = gasEstimate.CallGasLimit
	}
	if strings.TrimSpace(gasEstimate.VerificationGasLimit) == "" || strings.TrimSpace(gasEstimate.PreVerificationGas) == "" {
		return nil, newAppError(models.CodeSystemBusy, "bundler estimateUserOperationGas returned incomplete gas fields")
	}
	userOp.VerificationGasLimit = gasEstimate.VerificationGasLimit
	userOp.PreVerificationGas = gasEstimate.PreVerificationGas
	if strings.TrimSpace(gasEstimate.PaymasterVerificationGasLimit) != "" {
		userOp.PaymasterVerificationGasLimit = gasEstimate.PaymasterVerificationGasLimit
	}
	if strings.TrimSpace(gasEstimate.PaymasterPostOpGasLimit) != "" {
		userOp.PaymasterPostOpGasLimit = gasEstimate.PaymasterPostOpGasLimit
	}

	var authMap map[string]interface{}
	if alreadyDelegated {
		authMap = map[string]interface{}{
			"chainId": formatHexUint64(chainID),
			"address": delegator,
		}
	} else {
		authSignRes, err := p.svc.signEIP7702Authorization(ctx, wallet, chainID, delegator, txNonce)
		if err != nil {
			return nil, err
		}
		r, s, yParity, err := parseSignatureRSV(authSignRes.Signature)
		if err != nil {
			return nil, wrapSystemError(err)
		}
		authMap = map[string]interface{}{
			"chainId":   formatHexUint64(chainID),
			"address":   delegator,
			"nonce":     formatHexUint64(txNonce),
			"r":         r,
			"s":         s,
			"yParity":   yParity,
			"hash":      authSignRes.Hash,
			"signature": authSignRes.Signature,
		}
	}
	typedData, err := buildUserOperationTypedData(chainID, entryPoint, userOp)
	if err != nil {
		return nil, wrapSystemError(err)
	}
	userOpSignRes, err := p.svc.signEIP712TypedData(ctx, wallet, typedData)
	if err != nil {
		return nil, err
	}
	userOp.Signature = userOpSignRes.Signature

	userOpMap, err := structToMap(userOp)
	if err != nil {
		return nil, wrapSystemError(err)
	}

	var sendResp struct {
		TxCode string `json:"txCode"`
	}
	if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-invoke/evm/common/tx-send", map[string]interface{}{
		"entryPoint":    entryPoint,
		"userOperation": userOpMap,
		"eip7702Auth":   authMap,
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
		ToAddress:     transferToAddress,
		TokenAddress:  tokenAddress,
		TokenSymbol:   tokenSymbol,
		Amount:        requestAmount,
		TxHash:        sendResp.TxCode,
		Status:        models.StatusProcessing,
	}
	if err := p.svc.store.CreateTransaction(ctx, tx); err != nil {
		return nil, wrapSystemError(err)
	}
	go p.svc.notifyTransferOutResult(context.Background(), tx)
	return &models.TransferOutResponse{
		TransactionNo: tx.TransactionNo,
		RequestNo:     tx.RequestNo,
	}, nil
}

func (p *evmProvider) HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error {
	if req.Tx.Code == "" || normalizedNetwork(req.Tx.NetworkCode) != p.network {
		log.Infof("ignore evm tx callback code=%s network=%s", req.Tx.Code, req.Tx.NetworkCode)
		return nil
	}
	if _, err := p.svc.store.GetConnectorCallback(ctx, req.Tx.Code, connectorCallbackTypeTx); err == nil {
		log.Infof("ignore duplicate evm tx callback txCode=%s", req.Tx.Code)
		return nil
	} else if err != nil && !store.IsNotFound(err) {
		return err
	}

	rows, err := p.svc.store.ListTransactionsByTxHash(ctx, req.Tx.Code)
	if err != nil {
		return err
	}
	for i := range rows {
		tx := rows[i]
		if tx.Direction != models.DirectionOut {
			continue
		}
		nextStatus := models.StatusSuccess
		failReason := ""
		if strings.EqualFold(req.Tx.Status, "FAILED") {
			nextStatus = models.StatusFail
			failReason = "on-chain transaction failed"
		}
		if tx.Status == nextStatus && tx.Fee == req.Tx.Fee && tx.TxTime == req.Tx.Timestamp {
			continue
		}
		tx.Status = nextStatus
		tx.FailReason = failReason
		tx.Fee = req.Tx.Fee
		tx.TxTime = req.Tx.Timestamp
		if err := p.svc.store.UpdateTransaction(ctx, &tx); err != nil {
			return err
		}
		go p.svc.notifyTransferOutResult(context.Background(), &tx)
	}

	if err := p.handleIncomingNative(ctx, req); err != nil {
		return err
	}
	if err := p.handleIncomingToken(ctx, req); err != nil {
		return err
	}
	if err := p.svc.store.CreateConnectorCallback(ctx, &models.ConnectorCallbackEntity{
		TxCode:       req.Tx.Code,
		CallbackType: connectorCallbackTypeTx,
	}); err != nil && !isDuplicateCallbackError(err) {
		return err
	}
	return nil
}

func (p *evmProvider) HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error {
	if req.TxCode == "" || (normalizedNetwork(req.NetworkCode) != "" && normalizedNetwork(req.NetworkCode) != p.network) {
		return nil
	}
	if _, err := p.svc.store.GetConnectorCallback(ctx, req.TxCode, connectorCallbackTypeRollback); err == nil {
		log.Infof("ignore duplicate evm rollback callback txCode=%s", req.TxCode)
		return nil
	} else if err != nil && !store.IsNotFound(err) {
		return err
	}

	rows, err := p.svc.store.ListTransactionsByTxHash(ctx, req.TxCode)
	if err != nil {
		return err
	}
	for i := range rows {
		tx := rows[i]
		if tx.Status == models.StatusFail {
			continue
		}
		tx.Status = models.StatusFail
		tx.FailReason = "transaction reverted"
		if err := p.svc.store.UpdateTransaction(ctx, &tx); err != nil {
			return err
		}
		if tx.Direction == models.DirectionOut {
			go p.svc.notifyTransferOutResult(context.Background(), &tx)
		} else {
			go p.svc.notifyDeposit(context.Background(), &tx)
		}
	}
	if err := p.svc.store.CreateConnectorCallback(ctx, &models.ConnectorCallbackEntity{
		TxCode:       req.TxCode,
		CallbackType: connectorCallbackTypeRollback,
	}); err != nil && !isDuplicateCallbackError(err) {
		return err
	}
	return nil
}

func (p *evmProvider) subscribeAddress(ctx context.Context, address string) error {
	networkCode := p.svc.connectorNetworkCode(p.network)
	return p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data-subscribe/"+networkCode+"/address-subscribe", map[string]string{
		"address": address,
	}, nil)
}

func (p *evmProvider) listConnectorTokens(ctx context.Context) ([]evmToken, error) {
	var resp struct {
		Tokens []evmToken `json:"tokens"`
	}
	if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/token-list", map[string]string{
		"networkCode": p.svc.connectorNetworkCode(p.network),
	}, &resp); err != nil {
		return nil, wrapSystemError(err)
	}
	return resp.Tokens, nil
}

func (p *evmProvider) getConnectorToken(ctx context.Context, tokenCode string) (*evmToken, error) {
	var resp evmToken
	if err := p.svc.connectorPost(ctx, p.network, "/api/v1/inner/chain-data/evm/common/token-get", map[string]string{
		"code": tokenCode,
	}, &resp); err != nil {
		return nil, wrapSystemError(err)
	}
	return &resp, nil
}

func (p *evmProvider) findConnectorTokenByAddress(ctx context.Context, tokenAddress string) (*evmToken, error) {
	tokens, err := p.listConnectorTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if strings.EqualFold(token.Address(), tokenAddress) {
			tokenCopy := token
			return &tokenCopy, nil
		}
	}
	return nil, newAppError(models.CodeTokenUnsupported, "token not supported")
}

func (p *evmProvider) handleIncomingNative(ctx context.Context, req models.ConnectorTxCallbackRequest) error {
	if !strings.EqualFold(req.Tx.Status, "SUCCESS") || !validateEVMAddress(req.Tx.To) {
		return nil
	}
	wallet, err := p.svc.store.GetWalletByAddress(ctx, p.network, req.Tx.To)
	if err != nil {
		if store.IsNotFound(err) {
			log.Infof("ignore evm native incoming txHash=%s to=%s: wallet not found", req.Tx.Code, req.Tx.To)
			return nil
		}
		return err
	}
	if req.Tx.Amount == "" || req.Tx.Amount == "0" {
		log.Infof("ignore evm native incoming txHash=%s to=%s: zero amount", req.Tx.Code, req.Tx.To)
		return nil
	}
	return p.upsertIncomingTransaction(ctx, wallet, req.Tx.Code, models.TokenNative, p.svc.nativeTokenSymbol(p.network), req.Tx.Amount, req.Tx.From, req.Tx.To, req.Tx.Fee, req.Tx.Timestamp, models.StatusSuccess)
}

func (p *evmProvider) handleIncomingToken(ctx context.Context, req models.ConnectorTxCallbackRequest) error {
	for _, event := range req.TxEvents {
		if !p.isTokenTransferEvent(event.Type) {
			continue
		}
		toAddress, _ := event.Data["to"].(string)
		if !validateEVMAddress(toAddress) {
			continue
		}
		wallet, err := p.svc.store.GetWalletByAddress(ctx, p.network, toAddress)
		if err != nil {
			if store.IsNotFound(err) {
				log.Infof("ignore evm token incoming txHash=%s to=%s: wallet not found", req.Tx.Code, toAddress)
				continue
			}
			return err
		}

		tokenSymbol, _ := event.Data["tokenCode"].(string)
		tokenAddress, _ := event.Data["tokenAddress"].(string)
		if tokenAddress == "" {
			tokenAddress, _ = event.Data["contractAddress"].(string)
		}
		if tokenAddress == "" && tokenSymbol != "" {
			tokenMeta, err := p.findConnectorTokenByCode(ctx, tokenSymbol)
			if err != nil {
				return err
			}
			tokenAddress = tokenMeta.Address()
			tokenSymbol = tokenMeta.Code
		}
		if tokenAddress == "" {
			return newAppError(models.CodeTokenUnsupported, "token not supported")
		}
		if tokenSymbol == "" {
			tokenMeta, err := p.findConnectorTokenByAddress(ctx, tokenAddress)
			if err != nil {
				return err
			}
			tokenSymbol = tokenMeta.Code
		}

		amount := formatEventAmount(event.Data["amount"])
		fromAddress, _ := event.Data["from"].(string)
		if err := p.upsertIncomingTransaction(ctx, wallet, req.Tx.Code, tokenAddress, tokenSymbol, amount, fromAddress, toAddress, req.Tx.Fee, req.Tx.Timestamp, models.StatusSuccess); err != nil {
			return err
		}
	}
	return nil
}

func (p *evmProvider) upsertIncomingTransaction(ctx context.Context, wallet *models.WalletEntity, txHash string, tokenAddress string, tokenSymbol string, amount string, fromAddress string, toAddress string, fee string, txTime int64, status string) error {
	existing, err := p.svc.store.FindIncomingTransaction(ctx, wallet.WalletNo, txHash, tokenAddress)
	if err == nil {
		if existing.Status == status && existing.Amount == amount {
			return nil
		}
		existing.Status = status
		existing.Amount = amount
		existing.Fee = fee
		existing.TxTime = txTime
		existing.FromAddress = fromAddress
		existing.ToAddress = toAddress
		if err := p.svc.store.UpdateTransaction(ctx, existing); err != nil {
			return err
		}
		go p.svc.notifyDeposit(context.Background(), existing)
		return nil
	}
	if err != nil && !store.IsNotFound(err) {
		return err
	}

	tx := &models.TransactionEntity{
		TransactionNo: generateID("T"),
		RequestNo:     generateID("IR"),
		Direction:     models.DirectionIn,
		WalletNo:      wallet.WalletNo,
		Network:       wallet.Network,
		FromAddress:   fromAddress,
		ToAddress:     toAddress,
		TokenAddress:  tokenAddress,
		TokenSymbol:   tokenSymbol,
		Amount:        amount,
		Fee:           fee,
		TxHash:        txHash,
		Status:        status,
		TxTime:        txTime,
	}
	if err := p.svc.store.CreateTransaction(ctx, tx); err != nil {
		return err
	}
	go p.svc.notifyDeposit(context.Background(), tx)
	return nil
}

func (p *evmProvider) findConnectorTokenByCode(ctx context.Context, tokenCode string) (*evmToken, error) {
	token, err := p.getConnectorToken(ctx, tokenCode)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (p *evmProvider) isTokenTransferEvent(eventType string) bool {
	switch strings.ToUpper(strings.TrimSpace(eventType)) {
	case "RT_TRANSFER", "ERC20_TRANSFER", "TOKEN_TRANSFER":
		return true
	default:
		return false
	}
}

func (p *evmProvider) isEVMAddressType(chainType string) bool {
	chainType = normalizedNetwork(chainType)
	if chainType == "" {
		return false
	}
	if chainType == p.network || chainType == models.NetworkEVM {
		return true
	}
	switch chainType {
	case "eth", "ethereum", "polygon", "matic", "bsc", "arbitrum", "optimism", "base", "avalanche", "avax":
		return true
	default:
		return false
	}
}

func validateEVMAddress(address string) bool {
	return evmAddressPattern.MatchString(strings.TrimSpace(address))
}

func (t evmToken) Address() string {
	if strings.TrimSpace(t.ContractAddress) != "" {
		return strings.TrimSpace(t.ContractAddress)
	}
	return strings.TrimSpace(t.MintAddress)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p *evmProvider) prepareTransferBuild(ctx context.Context, fromAddress string) (*evmRPCClient, uint64, *big.Int, uint64, error) {
	connector := p.svc.connectorConfig(p.network)
	if connector == nil || strings.TrimSpace(connector.RPCEndpoint) == "" {
		return nil, 0, nil, 0, newAppError(models.CodeSystemBusy, "evm rpc endpoint is not configured")
	}
	rpcClient := newEVMRPCClient(p.svc.httpClient, connector.RPCEndpoint)
	nonce, err := rpcClient.getTransactionCount(ctx, fromAddress)
	if err != nil {
		return nil, 0, nil, 0, wrapSystemError(err)
	}
	gasPrice, err := rpcClient.gasPrice(ctx)
	if err != nil {
		return nil, 0, nil, 0, wrapSystemError(err)
	}
	chainID := connector.ChainID
	if chainID == 0 {
		chainID, err = rpcClient.chainID(ctx)
		if err != nil {
			return nil, 0, nil, 0, wrapSystemError(err)
		}
	}
	return rpcClient, chainID, gasPrice, nonce, nil
}

func (p *evmProvider) eip7702Enabled() bool {
	connector := p.svc.connectorConfig(p.network)
	return connector != nil && connector.EnableEIP7702
}

func (p *evmProvider) estimateNativeTransferGas(ctx context.Context, rpcClient *evmRPCClient, fromAddress string, toAddress string, gasPrice *big.Int, amount string) (uint64, error) {
	valueWei, err := decimalToBaseUnits(amount, 18)
	if err != nil {
		return 0, newAppError(models.CodeParamError, err.Error())
	}
	gasLimit, err := rpcClient.estimateGas(ctx, evmCallMsg{
		From:     strings.TrimSpace(fromAddress),
		To:       strings.TrimSpace(toAddress),
		GasPrice: formatHexBig(gasPrice),
		Value:    formatHexBig(valueWei),
		Data:     "0x",
	})
	if err != nil {
		return 0, wrapSystemError(err)
	}
	return gasLimit, nil
}

func (p *evmProvider) estimateERC20TransferGas(ctx context.Context, rpcClient *evmRPCClient, fromAddress string, contractAddress string, toAddress string, gasPrice *big.Int, amount string, decimals uint8) (uint64, error) {
	amountUnits, err := decimalToBaseUnits(amount, decimals)
	if err != nil {
		return 0, newAppError(models.CodeParamError, err.Error())
	}
	data, err := buildERC20TransferData(toAddress, amountUnits)
	if err != nil {
		return 0, newAppError(models.CodeParamError, err.Error())
	}
	gasLimit, err := rpcClient.estimateGas(ctx, evmCallMsg{
		From:     strings.TrimSpace(fromAddress),
		To:       strings.TrimSpace(contractAddress),
		GasPrice: formatHexBig(gasPrice),
		Value:    "0x0",
		Data:     data,
	})
	if err != nil {
		return 0, wrapSystemError(err)
	}
	return gasLimit, nil
}

func (p *evmProvider) lookupEIP7702Delegator(ctx context.Context, rpcClient *evmRPCClient, address string) (string, bool, error) {
	code, err := rpcClient.getCode(ctx, address)
	if err != nil {
		return "", false, wrapSystemError(err)
	}
	delegator, delegated, err := parseEIP7702DelegationIndicator(code)
	if err != nil {
		return "", false, wrapSystemError(err)
	}
	return delegator, delegated, nil
}

func (p *evmProvider) userOperationEstimateStateOverride(sender string, delegator string, alreadyDelegated bool) (evmStateOverride, error) {
	if alreadyDelegated {
		return nil, nil
	}
	code, err := buildEIP7702DelegationIndicatorCode(delegator)
	if err != nil {
		return nil, wrapSystemError(err)
	}
	return evmStateOverride{
		sender: {
			"code": code,
		},
	}, nil
}

func (p *evmProvider) userOperationCallGasLimit(native bool) uint64 {
	base := uint64(65000)
	if native {
		base = 21000
	}
	if native {
		if base < 25000 {
			base = 25000
		}
		return base * 5
	}
	if base < 65000 {
		base = 65000
	}
	return base * 3
}

func (p *evmProvider) newBundlerRPCClient() (*evmRPCClient, error) {
	connector := p.svc.connectorConfig(p.network)
	if connector == nil || strings.TrimSpace(connector.BundlerRPCEndpoint) == "" {
		return nil, newAppError(models.CodeSystemBusy, "bundler rpc endpoint is not configured")
	}
	return newEVMRPCClient(p.svc.httpClient, connector.BundlerRPCEndpoint), nil
}

func structToMap(v interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
