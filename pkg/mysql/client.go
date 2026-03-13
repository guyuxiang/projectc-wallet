package mysql

import (
	"database/sql"
	"errors"
	"time"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var db *gorm.DB

func Init(cfg *config.MySQL) (*gorm.DB, error) {
	if cfg == nil {
		return nil, errors.New("mysql config is nil")
	}
	if cfg.DSN == "" {
		return nil, errors.New("mysql dsn is empty")
	}

	conn, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	sqlDB, err := conn.DB()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = sqlDB.Close()
		}
	}()

	configurePool(sqlDB, cfg)
	if err = sqlDB.Ping(); err != nil {
		return nil, err
	}

	db = conn
	return db, nil
}

func DB() *gorm.DB {
	return db
}

func Close() error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

func configurePool(sqlDB *sql.DB, cfg *config.MySQL) {
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	sqlDB.SetConnMaxLifetime(time.Hour)
}
