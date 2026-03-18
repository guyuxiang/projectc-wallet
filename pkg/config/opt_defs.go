package config

var flagsOpts = []flagOpt{
	{
		optName:         FLAG_KEY_SERVER_HOST,
		optDefaultValue: "0.0.0.0",
		optUsage:        "server listen host",
	},
	{
		optName:         FLAG_KEY_SERVER_PORT,
		optDefaultValue: 8081,
		optUsage:        "server listen port",
	},
	{
		optName:         FLAG_KEY_GIN_MODE,
		optDefaultValue: "debug",
		optUsage:        "gin mode",
	},
	{
		optName:         FLAG_KEY_LOG_LEVEL,
		optDefaultValue: "info",
		optUsage:        "log level",
	},
	{
		optName:         FLAG_KEY_MYSQL_USERNAME,
		optDefaultValue: "",
		optUsage:        "mysql username",
	},
	{
		optName:         FLAG_KEY_MYSQL_PASSWORD,
		optDefaultValue: "",
		optUsage:        "mysql password",
	},
	{
		optName:         FLAG_KEY_MYSQL_HOST,
		optDefaultValue: "",
		optUsage:        "mysql host",
	},
	{
		optName:         FLAG_KEY_MYSQL_PORT,
		optDefaultValue: 0,
		optUsage:        "mysql port",
	},
	{
		optName:         FLAG_KEY_MYSQL_DATABASE,
		optDefaultValue: "projectc-custodial-wallet",
		optUsage:        "mysql database",
	},
	{
		optName:         FLAG_KEY_MYSQL_DSN,
		optDefaultValue: "",
		optUsage:        "mysql dsn",
	},
	{
		optName:         FLAG_KEY_MYSQL_MAX_IDLE,
		optDefaultValue: 10,
		optUsage:        "mysql max idle connections",
	},
	{
		optName:         FLAG_KEY_MYSQL_MAX_OPEN,
		optDefaultValue: 20,
		optUsage:        "mysql max open connections",
	},
	{
		optName:         FLAG_KEY_KMS_BASE_URL,
		optDefaultValue: "",
		optUsage:        "kms service base url",
	},
	{
		optName:         FLAG_KEY_KMS_USERNAME,
		optDefaultValue: "",
		optUsage:        "kms basic auth username",
	},
	{
		optName:         FLAG_KEY_KMS_PASSWORD,
		optDefaultValue: "",
		optUsage:        "kms basic auth password",
	},
	{
		optName:         FLAG_KEY_CONN_BASE_URL,
		optDefaultValue: "",
		optUsage:        "connector service base url",
	},
	{
		optName:         FLAG_KEY_CONN_USERNAME,
		optDefaultValue: "",
		optUsage:        "connector basic auth username",
	},
	{
		optName:         FLAG_KEY_CONN_PASSWORD,
		optDefaultValue: "",
		optUsage:        "connector basic auth password",
	},
	{
		optName:         FLAG_KEY_CONN_NETWORK,
		optDefaultValue: "solana",
		optUsage:        "connector network code",
	},
	{
		optName:         FLAG_KEY_CONN_NATIVE,
		optDefaultValue: "SOL",
		optUsage:        "connector native token symbol",
	},
	{
		optName:         FLAG_KEY_CB_DEPOSIT,
		optDefaultValue: "",
		optUsage:        "deposit callback url",
	},
	{
		optName:         FLAG_KEY_CB_OUT,
		optDefaultValue: "",
		optUsage:        "transfer out callback url",
	},
	{
		optName:         FLAG_KEY_CB_TIMEOUT,
		optDefaultValue: 10,
		optUsage:        "callback timeout seconds",
	},
	{
		optName:         FLAG_KEY_SOL_RPC,
		optDefaultValue: "",
		optUsage:        "solana rpc endpoint",
	},
	{
		optName:         FLAG_KEY_REQ_SIG_ENABLE,
		optDefaultValue: true,
		optUsage:        "enable request signature middleware",
	},
}
