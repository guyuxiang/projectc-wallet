package models

import "time"

const (
	CodeSuccess            = "0"
	CodeParamError         = "1001"
	CodeWalletNotFound     = "1002"
	CodeNetworkUnsupported = "1003"
	CodeAddressInvalid     = "1004"
	CodeTokenUnsupported   = "1005"
	CodeInsufficient       = "1006"
	CodeDuplicateRequest   = "1007"
	CodeTxNotFound         = "1008"
	CodeStatusInvalid      = "1009"
	CodeSignatureInvalid   = "1010"
	CodePermissionDenied   = "1011"
	CodeSystemBusy         = "1012"
	CodeCallbackFailed     = "1013"
	CodeTimeRangeInvalid   = "1014"
)

const (
	DirectionIn  = "IN"
	DirectionOut = "OUT"

	StatusProcessing = "PROCESSING"
	StatusSuccess    = "SUCCESS"
	StatusFail       = "FAIL"

	NetworkSolana = "solana"
	TokenNative   = "NATIVE"
)

type WalletInfoQueryRequest struct {
	WalletNo string `json:"walletNo" binding:"required"`
}

type WalletCreateRequest struct{}

type WalletCreateResponse struct {
	WalletNo   string `json:"walletNo"`
	Network    string `json:"network"`
	Address    string `json:"address"`
	KeystoreID string `json:"keystoreId"`
}

type WalletInfoQueryResponse struct {
	WalletNo string               `json:"walletNo"`
	Tokens   []WalletTokenBalance `json:"tokens"`
}

type WalletTokenBalance struct {
	TokenSymbol string `json:"tokenSymbol"`
	Balance     string `json:"balance"`
}

type TransferOutQueryRequest struct {
	WalletNo string `json:"walletNo" binding:"required"`
}

type TransferOutQueryResponse struct {
	WalletNo  string              `json:"walletNo"`
	AssetList []TransferableAsset `json:"assetList"`
}

type TransferableAsset struct {
	Network      string `json:"network"`
	TokenAddress string `json:"tokenAddress"`
	TokenSymbol  string `json:"tokenSymbol"`
}

type TransferOutRequest struct {
	RequestNo    string `json:"requestNo" binding:"required"`
	WalletNo     string `json:"walletNo" binding:"required"`
	Network      string `json:"network" binding:"required"`
	ToAddress    string `json:"toAddress" binding:"required"`
	TokenAddress string `json:"tokenAddress"`
	Amount       string `json:"amount" binding:"required"`
}

type TransferOutResponse struct {
	TransactionNo string `json:"transactionNo"`
	RequestNo     string `json:"requestNo"`
}

type TransactionQueryRequest struct {
	TransactionNo string `json:"transactionNo" binding:"required"`
}

type TransactionQueryResponse struct {
	TransactionNo string `json:"transactionNo"`
	Direction     string `json:"direction"`
	WalletNo      string `json:"walletNo"`
	Network       string `json:"network"`
	FromAddress   string `json:"fromAddress"`
	ToAddress     string `json:"toAddress"`
	TokenAddress  string `json:"tokenAddress"`
	TokenSymbol   string `json:"tokenSymbol"`
	Amount        string `json:"amount"`
	Fee           string `json:"fee"`
	TxHash        string `json:"txHash"`
	Status        string `json:"status"`
	FailReason    string `json:"failReason"`
	TxTime        int64  `json:"txTime"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime"`
}

type TransactionHistoryQueryRequest struct {
	WalletNo  string `json:"walletNo" binding:"required"`
	Direction string `json:"direction"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	PageSize  int    `json:"pageSize" binding:"required"`
	Cursor    int64  `json:"cursor"`
}

type TransactionHistoryQueryResponse struct {
	Items      []TransactionHistoryItem `json:"items"`
	NextCursor int64                    `json:"nextCursor"`
}

type TransactionHistoryItem struct {
	TransactionNo string `json:"transactionNo"`
	Direction     string `json:"direction"`
	WalletNo      string `json:"walletNo"`
	Network       string `json:"network"`
	FromAddress   string `json:"fromAddress"`
	ToAddress     string `json:"toAddress"`
	TokenAddress  string `json:"tokenAddress"`
	TokenSymbol   string `json:"tokenSymbol"`
	Amount        string `json:"amount"`
	Fee           string `json:"fee"`
	TxHash        string `json:"txHash"`
	Status        string `json:"status"`
	TxTime        int64  `json:"txTime"`
	CreatedTime   int64  `json:"createdTime"`
}

type DepositNotifyRequest struct {
	NotifyID      string `json:"notifyId"`
	TransactionNo string `json:"transactionNo"`
	WalletNo      string `json:"walletNo"`
	Network       string `json:"network"`
	Address       string `json:"address"`
	FromAddress   string `json:"fromAddress,omitempty"`
	TokenAddress  string `json:"tokenAddress,omitempty"`
	TokenSymbol   string `json:"tokenSymbol"`
	Amount        string `json:"amount"`
	TxHash        string `json:"txHash"`
	BlockHeight   string `json:"blockHeight,omitempty"`
	Confirmations int    `json:"confirmations,omitempty"`
	Status        string `json:"status"`
	TxTime        int64  `json:"txTime"`
	NotifyTime    int64  `json:"notifyTime"`
}

type TransferOutNotifyRequest struct {
	NotifyID      string `json:"notifyId"`
	TransactionNo string `json:"transactionNo"`
	RequestNo     string `json:"requestNo,omitempty"`
	WalletNo      string `json:"walletNo"`
	Network       string `json:"network"`
	ToAddress     string `json:"toAddress"`
	TokenAddress  string `json:"tokenAddress,omitempty"`
	TokenSymbol   string `json:"tokenSymbol"`
	Amount        string `json:"amount"`
	Fee           string `json:"fee,omitempty"`
	TxHash        string `json:"txHash,omitempty"`
	Status        string `json:"status"`
	FailReason    string `json:"failReason,omitempty"`
	TxTime        int64  `json:"txTime,omitempty"`
	NotifyTime    int64  `json:"notifyTime"`
}

type CallbackResponse struct {
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

type ConnectorChainTx struct {
	Code        string `json:"code"`
	NetworkCode string `json:"networkCode"`
	BlockNumber uint64 `json:"blockNumber"`
	Timestamp   int64  `json:"timestamp"`
	Status      string `json:"status"`
	From        string `json:"from"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
	Fee         string `json:"fee"`
}

type ConnectorChainEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type ConnectorTxCallbackRequest struct {
	Tx       ConnectorChainTx      `json:"tx"`
	TxEvents []ConnectorChainEvent `json:"txEvents"`
}

type ConnectorTxRollbackRequest struct {
	TxCode      string `json:"txCode"`
	NetworkCode string `json:"networkCode"`
}

type WalletEntity struct {
	ID              uint   `gorm:"primaryKey"`
	WalletNo        string `gorm:"size:64;uniqueIndex;not null"`
	Network         string `gorm:"size:32;index;not null"`
	Address         string `gorm:"size:128;index;not null"`
	KMSKeystoreID   string `gorm:"size:128;not null"`
	KMSPassword     string `gorm:"size:512;not null"`
	KMSKeyType      string `gorm:"size:32;not null"`
	AccountIndex    string `gorm:"size:32;default:''"`
	ChangeIndex     string `gorm:"size:32;default:''"`
	AddressIndex    string `gorm:"size:32;default:''"`
	DepositEnabled  bool   `gorm:"not null;default:true"`
	TransferEnabled bool   `gorm:"not null;default:true"`
	Status          string `gorm:"size:32;index;not null;default:'ACTIVE'"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (WalletEntity) TableName() string {
	return "wallets"
}

type TransactionEntity struct {
	ID            uint      `gorm:"primaryKey"`
	TransactionNo string    `gorm:"size:64;uniqueIndex;not null"`
	RequestNo     string    `gorm:"size:64;uniqueIndex"`
	Direction     string    `gorm:"size:8;index;not null"`
	WalletNo      string    `gorm:"size:64;index;not null"`
	Network       string    `gorm:"size:32;index;not null"`
	FromAddress   string    `gorm:"size:128;index"`
	ToAddress     string    `gorm:"size:128;index"`
	TokenAddress  string    `gorm:"size:128;index"`
	TokenSymbol   string    `gorm:"size:64"`
	Amount        string    `gorm:"size:64;not null"`
	Fee           string    `gorm:"size:64"`
	TxHash        string    `gorm:"size:128;index"`
	Status        string    `gorm:"size:32;index;not null"`
	FailReason    string    `gorm:"size:255"`
	TxTime        int64     `gorm:"index"`
	CreatedAt     time.Time `gorm:"index"`
	UpdatedAt     time.Time
}

func (TransactionEntity) TableName() string {
	return "wallet_transactions"
}

type ConnectorCallbackEntity struct {
	ID           uint      `gorm:"primaryKey"`
	TxCode       string    `gorm:"size:128;not null;uniqueIndex:idx_connector_callbacks_tx_type"`
	CallbackType string    `gorm:"size:32;not null;uniqueIndex:idx_connector_callbacks_tx_type"`
	CreatedAt    time.Time `gorm:"index"`
	UpdatedAt    time.Time
}

func (ConnectorCallbackEntity) TableName() string {
	return "connector_callbacks"
}
