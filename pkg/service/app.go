package service

import (
	"net/http"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
	"gorm.io/gorm"
)

type App struct {
	Wallet WalletService
}

var app *App

func InitApp(cfg *config.Config, db *gorm.DB) error {
	st := store.New(db)
	if err := st.AutoMigrate(); err != nil {
		return err
	}

	timeout := 10 * time.Second
	if cfg != nil && cfg.Callback != nil && cfg.Callback.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.Callback.TimeoutSeconds) * time.Second
	}
	httpClient := &http.Client{Timeout: timeout}

	svc := NewWalletService(cfg, st, httpClient)
	if err := svc.SyncSubscriptions(); err != nil {
		return err
	}

	app = &App{Wallet: svc}
	return nil
}

func GetApp() *App {
	return app
}
