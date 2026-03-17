package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type flagOpt struct {
	optName         string
	optDefaultValue interface{}
	optUsage        string
}

var c Config

func init() {

	// flag priority: cli > envvars > config > defaults
	for _, opt := range flagsOpts {
		viper.SetDefault(opt.optName, opt.optDefaultValue)
	}

	viper.SetConfigType("yaml")
	viper.SetConfigName(SERVICE_NAME)
	viper.AddConfigPath(fmt.Sprintf("/etc/%s/", SERVICE_NAME))   // path to look for the config file in
	viper.AddConfigPath(fmt.Sprintf("$HOME/.%s/", SERVICE_NAME)) // call multiple times to add many search paths
	viper.AddConfigPath("./etc/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		fmt.Fprintf(os.Stderr, "Fatal error config file: %s \n", err)
	}

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	_ = viper.BindEnv(FLAG_KEY_SOL_RPC, "SOLANA_RPC_ENDPOINT", "SOLANA_RPCENDPOINT")

	for _, opt := range flagsOpts {
		switch opt.optDefaultValue.(type) {
		case int:
			pflag.Int(opt.optName, opt.optDefaultValue.(int), opt.optUsage)
		case int64:
			pflag.Int64(opt.optName, opt.optDefaultValue.(int64), opt.optUsage)
		case uint64:
			pflag.Uint64(opt.optName, opt.optDefaultValue.(uint64), opt.optUsage)
		case string:
			pflag.String(opt.optName, opt.optDefaultValue.(string), opt.optUsage)
		case bool:
			pflag.Bool(opt.optName, opt.optDefaultValue.(bool), opt.optUsage)
		case []interface{}:
			continue
		default:
			continue
		}
	}

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	err = viper.Unmarshal(&c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to decode into struct, %v\n", err)
	}
}

func GetString(key string) string {
	return viper.GetString(key)
}

func GetInt(key string) int {
	return viper.GetInt(key)
}

func GetBool(key string) bool {
	return viper.GetBool(key)
}

func GetConfig() *Config {
	return &c
}
