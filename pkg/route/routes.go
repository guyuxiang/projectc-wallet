package route

import (
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/middleware"
	"os"

	"github.com/gin-gonic/gin"
	_ "github.com/guyuxiang/projectc-custodial-wallet/docs"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/controller"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
)

// @title Swagger projectc-custodial-wallet
// @version 0.1.0
// @description This is a projectc-custodial-wallet.
// @contact.name guyuxiang
// @contact.url https://guyuxiang.github.io
// @contact.email gu_yuxiang@qq.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /api/v1
func InstallRoutes(r *gin.Engine) {
	// Recovery middleware recovers from any panics and writes a 500 if there was one.
	r.Use(gin.Recovery())

	// /swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// a ping api test
	r.GET("/ping", controller.Ping)

	// get projectc-custodial-wallet version
	r.GET("/version", controller.Version)

	// config reload
	r.Any("/-/reload", func(c *gin.Context) {
		log.Info("===== Server Stop! Cause: Config Reload. =====")
		os.Exit(1)
	})

	rootGroup := r.Group("/api/v1")
	if config.GetBool(config.FLAG_KEY_REQ_SIG_ENABLE) {
		rootGroup.Use(middleware.RequestSignatureMiddleware())
	}

	adminGroup := r.Group("/api/v1/admin")
	adminGroup.Use(middleware.BasicAuthMiddleware())

	innerGroup := r.Group("/api/v1/inner")
	innerGroup.Use(middleware.BasicAuthMiddleware())

	{
		signatureKeyController := controller.NewSignatureKeyController()
		adminGroup.POST("/signature/key/upsert", signatureKeyController.Upsert)
	}

	{
		walletController := controller.NewWalletController()
		rootGroup.POST("/wallet/info/query", walletController.QueryWalletInfo)
		rootGroup.POST("/wallet/transfer/out/query", walletController.QueryTransferOutAssets)
		rootGroup.POST("/wallet/transfer/out", walletController.TransferOut)
		rootGroup.POST("/wallet/transaction/query", walletController.QueryTransaction)
		rootGroup.POST("/wallet/transaction/history/query", walletController.QueryHistory)

		innerGroup.POST("/wallet/create", walletController.CreateWallet)
		innerGroup.POST("/wallet/callback/tx", walletController.ReceiveTxCallback)
		innerGroup.POST("/wallet/callback/rollback", walletController.ReceiveRollbackCallback)
	}
}
