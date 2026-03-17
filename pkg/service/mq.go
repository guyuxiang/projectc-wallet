package service

import (
	"context"
	"strings"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
)

type txCallbackMessage struct {
	Tx       models.ConnectorChainTx      `json:"tx"`
	TxEvents []models.ConnectorChainEvent `json:"txEvents"`
}

type txRollbackMessage struct {
	TxCode      string `json:"txCode"`
	NetworkCode string `json:"networkCode"`
}

const (
	connectorCallbackTypeTx       = "TX"
	connectorCallbackTypeRollback = "ROLLBACK"
)

func (s *walletService) HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error {
	network := normalizedNetwork(req.Tx.NetworkCode)
	provider, err := s.provider(network)
	if err != nil {
		log.Infof("ignore tx callback for unsupported network=%s", req.Tx.NetworkCode)
		return nil
	}
	return provider.HandleTxCallback(ctx, req)
}

func (s *walletService) HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error {
	network := normalizedNetwork(req.NetworkCode)
	provider, err := s.provider(network)
	if err != nil {
		log.Infof("ignore rollback callback for unsupported network=%s", req.NetworkCode)
		return nil
	}
	return provider.HandleRollbackCallback(ctx, req)
}

func (s *walletService) handleTxCallback(msg txCallbackMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.HandleTxCallback(ctx, models.ConnectorTxCallbackRequest(msg))
}

func (s *walletService) handleRollback(msg txRollbackMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.HandleRollbackCallback(ctx, models.ConnectorTxRollbackRequest(msg))
}

func formatEventAmount(v interface{}) string {
	switch value := v.(type) {
	case float64:
		return formatFloat(value)
	case string:
		return normalizeAmountString(value)
	default:
		return "0.000000"
	}
}

func isDuplicateCallbackError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate entry")
}
