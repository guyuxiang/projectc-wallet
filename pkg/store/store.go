package store

import (
	"context"
	"errors"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"gorm.io/gorm"
)

type Store interface {
	AutoMigrate() error
	CreateWallet(ctx context.Context, wallet *models.WalletEntity) error
	GetWalletByNo(ctx context.Context, walletNo string) (*models.WalletEntity, error)
	GetWalletByAddress(ctx context.Context, network string, address string) (*models.WalletEntity, error)
	ListActiveWallets(ctx context.Context, network string) ([]models.WalletEntity, error)
	GetConnectorCallback(ctx context.Context, txCode string, callbackType string) (*models.ConnectorCallbackEntity, error)
	CreateConnectorCallback(ctx context.Context, callback *models.ConnectorCallbackEntity) error
	CreateTransaction(ctx context.Context, tx *models.TransactionEntity) error
	GetTransactionByNo(ctx context.Context, transactionNo string) (*models.TransactionEntity, error)
	GetTransactionByRequestNo(ctx context.Context, requestNo string) (*models.TransactionEntity, error)
	ListTransactionsByTxHash(ctx context.Context, txHash string) ([]models.TransactionEntity, error)
	FindIncomingTransaction(ctx context.Context, walletNo string, txHash string, tokenAddress string) (*models.TransactionEntity, error)
	UpdateTransaction(ctx context.Context, tx *models.TransactionEntity) error
	QueryHistory(ctx context.Context, req models.TransactionHistoryQueryRequest) ([]models.TransactionEntity, error)
}

func New(db *gorm.DB) Store {
	return &gormStore{db: db}
}

type gormStore struct {
	db *gorm.DB
}

func (s *gormStore) AutoMigrate() error {
	return s.db.AutoMigrate(&models.WalletEntity{}, &models.TransactionEntity{}, &models.ConnectorCallbackEntity{})
}

func (s *gormStore) CreateWallet(ctx context.Context, wallet *models.WalletEntity) error {
	return s.db.WithContext(ctx).Create(wallet).Error
}

func (s *gormStore) GetWalletByNo(ctx context.Context, walletNo string) (*models.WalletEntity, error) {
	var wallet models.WalletEntity
	err := s.db.WithContext(ctx).Where("wallet_no = ?", walletNo).First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

func (s *gormStore) GetWalletByAddress(ctx context.Context, network string, address string) (*models.WalletEntity, error) {
	var wallet models.WalletEntity
	err := s.db.WithContext(ctx).Where("network = ? AND address = ?", network, address).First(&wallet).Error
	if err != nil {
		return nil, err
	}
	return &wallet, nil
}

func (s *gormStore) ListActiveWallets(ctx context.Context, network string) ([]models.WalletEntity, error) {
	var wallets []models.WalletEntity
	q := s.db.WithContext(ctx).Where("status = ?", "ACTIVE")
	if network != "" {
		q = q.Where("network = ?", network)
	}
	if err := q.Find(&wallets).Error; err != nil {
		return nil, err
	}
	return wallets, nil
}

func (s *gormStore) GetConnectorCallback(ctx context.Context, txCode string, callbackType string) (*models.ConnectorCallbackEntity, error) {
	var callback models.ConnectorCallbackEntity
	err := s.db.WithContext(ctx).Where("tx_code = ? AND callback_type = ?", txCode, callbackType).First(&callback).Error
	if err != nil {
		return nil, err
	}
	return &callback, nil
}

func (s *gormStore) CreateConnectorCallback(ctx context.Context, callback *models.ConnectorCallbackEntity) error {
	return s.db.WithContext(ctx).Create(callback).Error
}

func (s *gormStore) CreateTransaction(ctx context.Context, tx *models.TransactionEntity) error {
	return s.db.WithContext(ctx).Create(tx).Error
}

func (s *gormStore) GetTransactionByNo(ctx context.Context, transactionNo string) (*models.TransactionEntity, error) {
	var tx models.TransactionEntity
	err := s.db.WithContext(ctx).Where("transaction_no = ?", transactionNo).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (s *gormStore) GetTransactionByRequestNo(ctx context.Context, requestNo string) (*models.TransactionEntity, error) {
	var tx models.TransactionEntity
	err := s.db.WithContext(ctx).Where("request_no = ?", requestNo).First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (s *gormStore) ListTransactionsByTxHash(ctx context.Context, txHash string) ([]models.TransactionEntity, error) {
	var out []models.TransactionEntity
	if err := s.db.WithContext(ctx).Where("tx_hash = ?", txHash).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *gormStore) FindIncomingTransaction(ctx context.Context, walletNo string, txHash string, tokenAddress string) (*models.TransactionEntity, error) {
	var tx models.TransactionEntity
	err := s.db.WithContext(ctx).
		Where("wallet_no = ? AND direction = ? AND tx_hash = ? AND token_address = ?", walletNo, models.DirectionIn, txHash, tokenAddress).
		First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (s *gormStore) UpdateTransaction(ctx context.Context, tx *models.TransactionEntity) error {
	return s.db.WithContext(ctx).Save(tx).Error
}

func (s *gormStore) QueryHistory(ctx context.Context, req models.TransactionHistoryQueryRequest) ([]models.TransactionEntity, error) {
	var items []models.TransactionEntity
	q := s.db.WithContext(ctx).Where("wallet_no = ?", req.WalletNo)
	if req.Direction != "" {
		q = q.Where("direction = ?", req.Direction)
	}
	if req.StartTime > 0 {
		q = q.Where("created_at >= ?", time.UnixMilli(req.StartTime))
	}
	if req.EndTime > 0 {
		q = q.Where("created_at <= ?", time.UnixMilli(req.EndTime))
	}
	if req.Cursor > 0 {
		q = q.Where("created_at < ?", time.UnixMilli(req.Cursor))
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	err := q.Order("created_at DESC").Limit(req.PageSize).Find(&items).Error
	return items, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
