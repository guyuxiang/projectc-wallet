package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/rabbitmq"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
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

func (s *walletService) StartMQConsumer() error {
	var err error
	s.mqOnce.Do(func() {
		var deliveries <-chan rabbitmq.Delivery
		deliveries, err = rabbitmq.ConsumeWalletCallbacks("projectc-custodial-wallet-consumer")
		if err != nil {
			return
		}
		log.Infof("wallet mq consumer started consumer=%s", "projectc-custodial-wallet-consumer")
		go func() {
			for delivery := range deliveries {
				log.Infof("wallet mq delivery received type=%s body=%s", delivery.Type, string(delivery.Body))
				if handleErr := s.handleMQDelivery(delivery); handleErr != nil {
					log.Warningf("handle mq delivery failed type=%s err=%v", delivery.Type, handleErr)
					_ = delivery.Nack(false, true)
					continue
				}
				_ = delivery.Ack(false)
			}
		}()
	})
	return err
}

func (s *walletService) handleMQDelivery(delivery rabbitmq.Delivery) error {
	switch delivery.Type {
	case "rollback":
		var msg txRollbackMessage
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			return err
		}
		return s.handleRollback(msg)
	default:
		var msg txCallbackMessage
		if err := json.Unmarshal(delivery.Body, &msg); err != nil {
			return err
		}
		return s.handleTxCallback(msg)
	}
}

func (s *walletService) HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error {
	return s.handleTxCallback(txCallbackMessage(req))
}

func (s *walletService) HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error {
	return s.handleRollback(txRollbackMessage(req))
}

func (s *walletService) handleTxCallback(msg txCallbackMessage) error {
	if msg.Tx.Code == "" || strings.ToLower(msg.Tx.NetworkCode) != s.networkCode() {
		log.Infof("ignore mq tx callback code=%s network=%s", msg.Tx.Code, msg.Tx.NetworkCode)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := s.store.GetConnectorCallback(ctx, msg.Tx.Code, connectorCallbackTypeTx); err == nil {
		log.Infof("ignore duplicate tx callback txCode=%s", msg.Tx.Code)
		return nil
	} else if err != nil && !store.IsNotFound(err) {
		return err
	}

	rows, err := s.store.ListTransactionsByTxHash(ctx, msg.Tx.Code)
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
		if strings.EqualFold(msg.Tx.Status, "FAILED") {
			nextStatus = models.StatusFail
			failReason = "on-chain transaction failed"
		}
		if tx.Status == nextStatus && tx.Fee == msg.Tx.Fee && tx.TxTime == msg.Tx.Timestamp {
			continue
		}
		tx.Status = nextStatus
		tx.FailReason = failReason
		tx.Fee = msg.Tx.Fee
		tx.TxTime = msg.Tx.Timestamp
		if err := s.store.UpdateTransaction(ctx, &tx); err != nil {
			return err
		}
		go s.notifyTransferOutResult(context.Background(), &tx)
	}

	if err := s.handleIncomingNative(ctx, msg); err != nil {
		return err
	}
	if err := s.handleIncomingToken(ctx, msg); err != nil {
		return err
	}
	if err := s.store.CreateConnectorCallback(ctx, &models.ConnectorCallbackEntity{
		TxCode:       msg.Tx.Code,
		CallbackType: connectorCallbackTypeTx,
	}); err != nil && !isDuplicateCallbackError(err) {
		return err
	}
	return nil
}

func (s *walletService) handleIncomingNative(ctx context.Context, msg txCallbackMessage) error {
	if !strings.EqualFold(msg.Tx.Status, "SUCCESS") {
		return nil
	}
	wallet, err := s.store.GetWalletByAddress(ctx, s.networkCode(), msg.Tx.To)
	if err != nil {
		if store.IsNotFound(err) {
			log.Infof("ignore native incoming txHash=%s to=%s: wallet not found", msg.Tx.Code, msg.Tx.To)
			return nil
		}
		return err
	}
	if msg.Tx.Amount == "" || msg.Tx.Amount == "0" {
		log.Infof("ignore native incoming txHash=%s to=%s: zero amount", msg.Tx.Code, msg.Tx.To)
		return nil
	}
	return s.upsertIncomingTransaction(ctx, wallet, msg.Tx.Code, models.TokenNative, s.nativeTokenSymbol(), msg.Tx.Amount, msg.Tx.From, msg.Tx.To, msg.Tx.Fee, msg.Tx.Timestamp, models.StatusSuccess)
}

func (s *walletService) handleIncomingToken(ctx context.Context, msg txCallbackMessage) error {
	for _, event := range msg.TxEvents {
		if event.Type != "RT_TRANSFER" {
			continue
		}
		toAddress, _ := event.Data["to"].(string)
		if toAddress == "" {
			continue
		}
		wallet, err := s.store.GetWalletByAddress(ctx, s.networkCode(), toAddress)
		if err != nil {
			if store.IsNotFound(err) {
				log.Infof("ignore token incoming txHash=%s to=%s: wallet not found", msg.Tx.Code, toAddress)
				continue
			}
			return err
		}
		tokenCode, _ := event.Data["tokenCode"].(string)
		token, err := s.getConnectorToken(ctx, tokenCode)
		if err != nil {
			return err
		}
		amount := formatEventAmount(event.Data["amount"])
		fromAddress, _ := event.Data["from"].(string)
		if err := s.upsertIncomingTransaction(ctx, wallet, msg.Tx.Code, token.MintAddress, token.Code, amount, fromAddress, toAddress, msg.Tx.Fee, msg.Tx.Timestamp, models.StatusSuccess); err != nil {
			return err
		}
	}
	return nil
}

func (s *walletService) upsertIncomingTransaction(ctx context.Context, wallet *models.WalletEntity, txHash string, tokenAddress string, tokenSymbol string, amount string, fromAddress string, toAddress string, fee string, txTime int64, status string) error {
	existing, err := s.store.FindIncomingTransaction(ctx, wallet.WalletNo, txHash, tokenAddress)
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
		if err := s.store.UpdateTransaction(ctx, existing); err != nil {
			return err
		}
		go s.notifyDeposit(context.Background(), existing)
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
	if err := s.store.CreateTransaction(ctx, tx); err != nil {
		return err
	}
	go s.notifyDeposit(context.Background(), tx)
	return nil
}

func (s *walletService) handleRollback(msg txRollbackMessage) error {
	if msg.TxCode == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := s.store.GetConnectorCallback(ctx, msg.TxCode, connectorCallbackTypeRollback); err == nil {
		log.Infof("ignore duplicate rollback callback txCode=%s", msg.TxCode)
		return nil
	} else if err != nil && !store.IsNotFound(err) {
		return err
	}

	rows, err := s.store.ListTransactionsByTxHash(ctx, msg.TxCode)
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
		if err := s.store.UpdateTransaction(ctx, &tx); err != nil {
			return err
		}
		if tx.Direction == models.DirectionOut {
			go s.notifyTransferOutResult(context.Background(), &tx)
		} else {
			go s.notifyDeposit(context.Background(), &tx)
		}
	}
	if err := s.store.CreateConnectorCallback(ctx, &models.ConnectorCallbackEntity{
		TxCode:       msg.TxCode,
		CallbackType: connectorCallbackTypeRollback,
	}); err != nil && !isDuplicateCallbackError(err) {
		return err
	}
	return nil
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
