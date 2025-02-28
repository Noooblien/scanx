package main

import (
	"flag"
	"os"
	"time"

	"github.com/Noooblien/scanx/internal/api"
	"github.com/Noooblien/scanx/internal/config"
	"github.com/Noooblien/scanx/internal/db"
	"github.com/Noooblien/scanx/internal/indexer"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var Version string = "development"

func main() {
	// Setup colorful zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel) // Set to INFO
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	configFile := flag.String("config", "config.json", "Path to config file")
	apiOnly := flag.Bool("api-only", false, "Run only the API without indexing")
	flag.Parse()

	log.Info().Str("version", Version).Msg("Starting ScanX Multi-Chain Indexer")

	cfg := config.LoadConfig(*configFile)
	db.InitPostgres(cfg.DBConn)
	db.DB.SetMaxOpenConns(150)
	db.DB.SetMaxIdleConns(50)

	if !*apiOnly {
		for _, chain := range cfg.Chains {
			log.Info().Str("chain", chain.Name).Int64("chain_id", chain.ChainID).Msg("Initializing indexer")
			idx, err := indexer.NewIndexer(chain.RPCURL, chain.Name, chain.ChainID)
			if err != nil {
				log.Error().Err(err).Str("chain", chain.Name).Msg("Failed to initialize indexer")
				continue
			}
			go idx.Start(chain.StartBlock)
			log.Info().Str("chain", chain.Name).Msg("Started indexer")
		}
	}

	api.StartAPI(cfg.Chains)
}
