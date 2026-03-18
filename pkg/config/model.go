package config

import "fmt"

type Config struct {
	Server     *Server               `yaml:"server"`
	Auth       *Auth                 `yaml:"auth"`
	Gin        *Gin                  `yaml:"gin"`
	Log        *Log                  `yaml:"log"`
	MySQL      *MySQL                `yaml:"mysql"`
	KMS        *KMS                  `yaml:"kms"`
	Connector  *Connector            `yaml:"connector"`
	Connectors map[string]*Connector `yaml:"connectors"`
	Callback   *Callback             `yaml:"callback"`
}

type Server struct {
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
	Version string `yaml:"version"`
}

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Gin struct {
	Mode string `yaml:"mode"`
}

type Log struct {
	Level string `yaml:"level"`
}

type MySQL struct {
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Database     string `yaml:"database"`
	DSN          string `yaml:"dsn"`
	MaxIdleConns int    `yaml:"maxIdleConns"`
	MaxOpenConns int    `yaml:"maxOpenConns"`
}

func (m *MySQL) EffectiveDSN() string {
	if m == nil {
		return ""
	}
	if m.DSN != "" {
		return m.DSN
	}
	if m.Username == "" || m.Host == "" || m.Port <= 0 || m.Database == "" {
		return ""
	}

	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.Username,
		m.Password,
		m.Host,
		m.Port,
		m.Database,
	)
}

type KMS struct {
	BaseURL  string `yaml:"baseUrl"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Connector struct {
	BaseURL            string `yaml:"baseUrl"`
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	Driver             string `yaml:"driver"`
	NetworkCode        string `yaml:"networkCode"`
	NativeTokenSymbol  string `yaml:"nativeTokenSymbol"`
	RPCEndpoint        string `yaml:"rpcEndpoint"`
	ChainID            uint64 `yaml:"chainId"`
	BundlerRPCEndpoint string `yaml:"bundlerRpcEndpoint"`
	EntryPoint         string `yaml:"entryPoint"`
	EnableEIP7702      bool   `yaml:"enableEip7702"`
	EIP7702Delegator   string `yaml:"eip7702Delegator"`
}

type Callback struct {
	DepositURL     string `yaml:"depositUrl"`
	TransferOutURL string `yaml:"transferOutUrl"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
}
