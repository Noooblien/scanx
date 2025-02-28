package db

import (
	"database/sql"
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var DB *sqlx.DB

func InitPostgres(connStr string) {
	defaultConnStr := connStr[:strings.LastIndex(connStr, "/")] + "/postgres?sslmode=disable"
	defaultDB, err := sql.Open("postgres", defaultConnStr)
	if err != nil {
		log.Fatal("Failed to connect to default postgres DB:", err)
	}
	defer defaultDB.Close()

	var exists bool
	err = defaultDB.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = 'scanx')").Scan(&exists)
	if err != nil {
		log.Fatal("Failed to check if scanx database exists:", err)
	}

	if !exists {
		_, err = defaultDB.Exec("CREATE DATABASE scanx")
		if err != nil {
			log.Fatal("Failed to create scanx database:", err)
		}
		log.Println("Created scanx database")
	}

	DB, err = sqlx.Connect("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to scanx DB:", err)
	}
	log.Println("Postgres initialized")
}

func InitChainSchema(chainName string) {
	schema := `
    CREATE SCHEMA IF NOT EXISTS ` + chainName + `;

    CREATE TABLE IF NOT EXISTS ` + chainName + `.blocks (
        id BIGINT PRIMARY KEY,
        hash BYTEA NOT NULL,
        parent_hash BYTEA NOT NULL,
        timestamp BIGINT NOT NULL,
        miner TEXT NOT NULL,
        gas_limit BIGINT NOT NULL,
        gas_used BIGINT NOT NULL,
        base_fee_per_gas NUMERIC,
        difficulty NUMERIC NOT NULL,
        total_difficulty NUMERIC NOT NULL,
        transactions_root BYTEA NOT NULL,
        receipts_root BYTEA NOT NULL,
        state_root BYTEA NOT NULL,
        size INT NOT NULL,
        nonce BYTEA NOT NULL,
        logs_bloom BYTEA NOT NULL,
        extra_data BYTEA,
        tx_count INT NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_blocks_hash ON ` + chainName + `.blocks(hash);

    CREATE TABLE IF NOT EXISTS ` + chainName + `.transactions (
        hash BYTEA PRIMARY KEY,
        block_number BIGINT NOT NULL,
        block_hash BYTEA NOT NULL,
        "from" TEXT NOT NULL,
        "to" TEXT,
        value NUMERIC NOT NULL,
        gas BIGINT NOT NULL,
        gas_used BIGINT NOT NULL,
        gas_price NUMERIC NOT NULL,
        max_fee_per_gas NUMERIC,
        max_priority_fee_per_gas NUMERIC,
        effective_gas_price NUMERIC NOT NULL,
        nonce BIGINT NOT NULL,
        input BYTEA,
        type SMALLINT NOT NULL,
        transaction_index INT NOT NULL,
        chain_id NUMERIC NOT NULL,
        v BYTEA NOT NULL,
        r BYTEA NOT NULL,
        s BYTEA NOT NULL,
        timestamp BIGINT NOT NULL,
        logs_bloom BYTEA NOT NULL,
        logs JSONB,
        status BOOLEAN NOT NULL,
        contract_address TEXT,
        token_transfer BOOLEAN DEFAULT FALSE,
        nft_transfer BOOLEAN DEFAULT FALSE,
        internal_txs BOOLEAN DEFAULT FALSE,
        FOREIGN KEY (block_number) REFERENCES ` + chainName + `.blocks(id)
    );
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_transactions_block_number ON ` + chainName + `.transactions(block_number);
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_transactions_from ON ` + chainName + `.transactions("from");
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_transactions_to ON ` + chainName + `.transactions("to");

    CREATE TABLE IF NOT EXISTS ` + chainName + `.internal_transactions (
        id SERIAL PRIMARY KEY,
        tx_hash BYTEA NOT NULL,
        "from" TEXT NOT NULL,
        "to" TEXT NOT NULL,
        value NUMERIC NOT NULL,
        call_type TEXT NOT NULL,
        gas BIGINT NOT NULL,
        gas_used BIGINT NOT NULL,
        input BYTEA,
        output BYTEA,
        trace_address JSONB,
        FOREIGN KEY (tx_hash) REFERENCES ` + chainName + `.transactions(hash)
    );
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_internal_transactions_tx_hash ON ` + chainName + `.internal_transactions(tx_hash);

    CREATE TABLE IF NOT EXISTS ` + chainName + `.contracts (
        address TEXT PRIMARY KEY,
        creator TEXT NOT NULL,
        creation_block BIGINT NOT NULL,
        creation_tx_hash BYTEA NOT NULL,
        bytecode BYTEA,
        FOREIGN KEY (creation_block) REFERENCES ` + chainName + `.blocks(id),
        FOREIGN KEY (creation_tx_hash) REFERENCES ` + chainName + `.transactions(hash)
    );
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_contracts_creation_block ON ` + chainName + `.contracts(creation_block);

    CREATE TABLE IF NOT EXISTS ` + chainName + `.addresses (
        address TEXT PRIMARY KEY,
        balance NUMERIC NOT NULL,
        is_contract BOOLEAN NOT NULL,
        bytecode BYTEA,
        last_updated_block BIGINT NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_` + chainName + `_addresses_balance ON ` + chainName + `.addresses(balance);

    CREATE TABLE IF NOT EXISTS ` + chainName + `.stats (
        id SERIAL PRIMARY KEY,
        timestamp BIGINT NOT NULL,
        total_blocks BIGINT NOT NULL,
        total_txs BIGINT NOT NULL,
        total_gas_used BIGINT NOT NULL,
        avg_tps FLOAT NOT NULL,
        avg_gas_per_tx BIGINT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS ` + chainName + `.sync_state (
        id SERIAL PRIMARY KEY,
        last_indexed_block BIGINT NOT NULL DEFAULT 0
    );`

	// Execute each statement with error logging
	statements := strings.Split(schema, ";")
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// log.Printf("Executing statement %d for chain %s: %s", i+1, chainName, stmt)
		_, err := DB.Exec(stmt)
		if err != nil {
			log.Printf("Failed to execute schema statement %d for chain %s: %v", i+1, chainName, err)
			panic(err) // Panic here to stop and debug
		}
	}

	var count int
	err := DB.Get(&count, "SELECT COUNT(*) FROM "+chainName+".sync_state")
	if err != nil {
		log.Printf("Failed to check sync_state for chain %s: %v", chainName, err)
		panic(err)
	}
	if count == 0 {
		_, err = DB.Exec("INSERT INTO " + chainName + ".sync_state (last_indexed_block) VALUES (0)")
		if err != nil {
			log.Printf("Failed to initialize sync_state for chain %s: %v", chainName, err)
			panic(err)
		}
	}
	log.Printf("Initialized schema for chain %s", chainName)
}

func GetLastIndexedBlock(chainName string) int64 {
	var lastBlock int64
	err := DB.Get(&lastBlock, "SELECT last_indexed_block FROM "+chainName+".sync_state ORDER BY id DESC LIMIT 1")
	if err != nil {
		log.Printf("Failed to get last indexed block for chain %s, defaulting to 0: %v", chainName, err)
		return 0
	}
	return lastBlock
}

func SetLastIndexedBlock(chainName string, block int64) {
	_, err := DB.Exec("UPDATE "+chainName+".sync_state SET last_indexed_block = $1", block)
	if err != nil {
		log.Printf("Failed to set last indexed block for chain %s: %v", chainName, err)
	}
}
