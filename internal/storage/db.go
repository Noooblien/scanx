package storage

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
)

var DB *pgx.Conn

func ConnectDB(dbURL string) {
	var err error
	DB, err = pgx.Connect(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connected!")
}

func InitDB() {
	_, err := DB.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS blocks (
			block_number BIGINT PRIMARY KEY,
			block_hash TEXT UNIQUE,
			parent_hash TEXT,
			timestamp BIGINT,
			miner TEXT,
			gas_used BIGINT
		);
		CREATE TABLE IF NOT EXISTS transactions (
			tx_hash TEXT PRIMARY KEY,
			block_number BIGINT REFERENCES blocks(block_number),
			from_address TEXT,
			to_address TEXT,
			value NUMERIC,
			gas_used BIGINT,
			input_data TEXT,
			status BOOLEAN
		);
		CREATE TABLE IF NOT EXISTS logs (
			log_id SERIAL PRIMARY KEY,
			tx_hash TEXT REFERENCES transactions(tx_hash),
			contract_address TEXT,
			topic TEXT,
			data TEXT
		);
	`)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized!")
}