package config

import (
	"encoding/json"
	"log"
	"os"
)

type ChainConfig struct {
	Name       string `json:"name"`
	RPCURL     string `json:"rpc_url"`
	ChainID    int64  `json:"chain_id"`
	StartBlock int64  `json:"start_block"`
}

type Config struct {
	DBConn   string        `json:"db_conn"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	Chains   []ChainConfig `json:"chains"`
}

func LoadConfig(file string) Config {
	data, err := os.ReadFile(file)
	if err != nil {
		log.Fatal("Failed to read config file:", err)
	}
	var cfg Config
	json.Unmarshal(data, &cfg)
	return cfg
}
