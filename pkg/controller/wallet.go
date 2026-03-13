package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-wallet/pkg/models"
	"github.com/guyuxiang/projectc-wallet/pkg/service"
)

type WalletController interface {
	CreateWallet(c *gin.Context)
	QueryWalletInfo(c *gin.Context)
	QueryTransferOutAssets(c *gin.Context)
	TransferOut(c *gin.Context)
	QueryTransaction(c *gin.Context)
	QueryHistory(c *gin.Context)
	ReceiveTxCallback(c *gin.Context)
	ReceiveRollbackCallback(c *gin.Context)
}

func NewWalletController() WalletController {
	return &walletController{
		service: service.GetApp().Wallet,
	}
}

type walletController struct {
	service service.WalletService
}

// CreateWallet godoc
// @Summary CreateWallet
// @Description Create a centralized wallet and subscribe its Solana address.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.WalletCreateRequest false "Create wallet request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/create [post]
func (wc *walletController) CreateWallet(c *gin.Context) {
	req := models.WalletCreateRequest{}
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	resp, err := wc.service.CreateWallet(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

// QueryWalletInfo godoc
// @Summary QueryWalletInfo
// @Description Query wallet token balances.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.WalletInfoQueryRequest true "Wallet info query request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/info/query [post]
func (wc *walletController) QueryWalletInfo(c *gin.Context) {
	var req models.WalletInfoQueryRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := wc.service.QueryWalletInfo(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

// QueryTransferOutAssets godoc
// @Summary QueryTransferOutAssets
// @Description Query transferable assets of a wallet, including native token and configured SPL tokens.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.TransferOutQueryRequest true "Transfer out capability query request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/transfer/out/query [post]
func (wc *walletController) QueryTransferOutAssets(c *gin.Context) {
	var req models.TransferOutQueryRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := wc.service.QueryTransferOutAssets(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

// TransferOut godoc
// @Summary TransferOut
// @Description Submit a wallet transfer-out request for native SOL or configured SPL tokens.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.TransferOutRequest true "Transfer out request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/transfer/out [post]
func (wc *walletController) TransferOut(c *gin.Context) {
	var req models.TransferOutRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := wc.service.TransferOut(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

// QueryTransaction godoc
// @Summary QueryTransaction
// @Description Query a single transaction by transactionNo.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.TransactionQueryRequest true "Transaction query request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/transaction/query [post]
func (wc *walletController) QueryTransaction(c *gin.Context) {
	var req models.TransactionQueryRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := wc.service.QueryTransaction(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

// QueryHistory godoc
// @Summary QueryHistory
// @Description Query wallet transaction history by cursor and time range.
// @Tags Wallet
// @Accept json
// @Produce json
// @Param request body models.TransactionHistoryQueryRequest true "Transaction history query request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /wallet/transaction/history/query [post]
func (wc *walletController) QueryHistory(c *gin.Context) {
	var req models.TransactionHistoryQueryRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := wc.service.QueryHistory(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}

func (wc *walletController) ReceiveTxCallback(c *gin.Context) {
	var req models.ConnectorTxCallbackRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := wc.service.HandleTxCallback(c.Request.Context(), req); err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: struct{}{}})
}

func (wc *walletController) ReceiveRollbackCallback(c *gin.Context) {
	var req models.ConnectorTxRollbackRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := wc.service.HandleRollbackCallback(c.Request.Context(), req); err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: struct{}{}})
}

func bindJSON(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, models.Response{
			Code:    models.CodeParamError,
			Message: err.Error(),
			Data:    struct{}{},
		})
		return false
	}
	return true
}

func writeAppError(c *gin.Context, err error) {
	if appErr, ok := err.(*service.AppError); ok {
		c.JSON(http.StatusOK, models.Response{
			Code:    appErr.Code,
			Message: appErr.Message,
			Data:    struct{}{},
		})
		return
	}
	c.JSON(http.StatusOK, models.Response{
		Code:    models.CodeSystemBusy,
		Message: err.Error(),
		Data:    struct{}{},
	})
}
