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
		optName:         FLAG_KEY_RABBIT_URL,
		optDefaultValue: "",
		optUsage:        "rabbitmq connection url",
	},
	{
		optName:         FLAG_KEY_RABBIT_VHOST,
		optDefaultValue: "",
		optUsage:        "rabbitmq virtual host",
	},
	{
		optName:         FLAG_KEY_RABBIT_EXCH,
		optDefaultValue: "tx_callback_fanout_exchange",
		optUsage:        "rabbitmq exchange",
	},
	{
		optName:         FLAG_KEY_RABBIT_CANCEL,
		optDefaultValue: "tx_callback_cancel_fanout_exchange",
		optUsage:        "rabbitmq cancel exchange",
	},
	{
		optName:         FLAG_KEY_RABBIT_TYPE,
		optDefaultValue: "fanout",
		optUsage:        "rabbitmq exchange type",
	},
	{
		optName:         FLAG_KEY_RABBIT_QUEUE,
		optDefaultValue: "projectc-custodial-wallet.queue",
		optUsage:        "rabbitmq queue",
	},
	{
		optName:         FLAG_KEY_RABBIT_ROUTING,
		optDefaultValue: "",
		optUsage:        "rabbitmq routing key",
	},
	{
		optName:         FLAG_KEY_SIGN_APP_ID,
		optDefaultValue: "",
		optUsage:        "request signature app id",
	},
	{
		optName:         FLAG_KEY_SIGN_SECRET,
		optDefaultValue: "",
		optUsage:        "request signature secret",
	},
	{
		optName:         FLAG_KEY_SIGN_SKEW,
		optDefaultValue: int64(300000),
		optUsage:        "request signature max skew millis",
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
		optName:         FLAG_KEY_SOL_CU_PRICE,
		optDefaultValue: 0,
		optUsage:        "solana compute unit price",
	},
}
