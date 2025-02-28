package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Noooblien/scanx/internal/config"
	"github.com/Noooblien/scanx/internal/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all origins for now
}

func StartAPI(chains []config.ChainConfig) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1"})

	// WebSocket connections per chain
	wsConns := make(map[string]map[*websocket.Conn]bool)
	for _, chain := range chains {
		wsConns[chain.Name] = make(map[*websocket.Conn]bool)
	}

	// Broadcast new blocks to WebSocket clients
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			for _, chain := range chains {
				var latestBlock struct {
					ID        int64  `db:"id" json:"id"`
					Hash      []byte `db:"hash" json:"hash"`
					Timestamp int64  `db:"timestamp" json:"timestamp"`
					TxCount   int    `db:"tx_count" json:"tx_count"`
				}
				err := db.DB.Get(&latestBlock, fmt.Sprintf(`
                    SELECT id, hash, timestamp, tx_count 
                    FROM %s.blocks 
                    ORDER BY id DESC 
                    LIMIT 1`, chain.Name))
				if err != nil {
					continue
				}
				latestBlock.Hash = common.Hex2Bytes(common.Bytes2Hex(latestBlock.Hash))
				blockJSON, _ := json.Marshal(latestBlock)
				conns := wsConns[chain.Name]
				for conn := range conns {
					if err := conn.WriteMessage(websocket.TextMessage, blockJSON); err != nil {
						delete(conns, conn)
						conn.Close()
					}
				}
			}
		}
	}()

	for _, chain := range chains {
		chainName := chain.Name

		// Paginated transactions
		r.GET("/"+chainName+"/transactions", func(c *gin.Context) {
			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
			offset := (page - 1) * limit
			var txs []struct {
				Hash        []byte `db:"hash" json:"hash"`
				BlockNumber int64  `db:"block_number" json:"block_number"`
				From        string `db:"from" json:"from"`
				To          string `db:"to" json:"to"`
				Value       string `db:"value" json:"value"`
				GasUsed     int64  `db:"gas_used" json:"gas_used"`
				Timestamp   int64  `db:"timestamp" json:"timestamp"`
			}
			err := db.DB.Select(&txs, fmt.Sprintf(`
                SELECT hash, block_number, "from", "to", value, gas_used, timestamp 
                FROM %s.transactions 
                ORDER BY block_number DESC 
                LIMIT $1 OFFSET $2`, chainName), limit, offset)
			if err != nil {
				c.JSON(500, gin.H{"error": "Failed to fetch transactions"})
				return
			}
			for i := range txs {
				txs[i].Hash = common.Hex2Bytes(common.Bytes2Hex(txs[i].Hash))
			}
			c.JSON(200, txs)
		})

		// Transaction by hash
		r.GET("/"+chainName+"/transactions/:hash", func(c *gin.Context) {
			hash := c.Param("hash")
			var tx struct {
				Hash        []byte `db:"hash" json:"hash"`
				BlockNumber int64  `db:"block_number" json:"block_number"`
				From        string `db:"from" json:"from"`
				To          string `db:"to" json:"to"`
				Value       string `db:"value" json:"value"`
				GasUsed     int64  `db:"gas_used" json:"gas_used"`
				Timestamp   int64  `db:"timestamp" json:"timestamp"`
			}
			err := db.DB.Get(&tx, fmt.Sprintf(`
                SELECT hash, block_number, "from", "to", value, gas_used, timestamp 
                FROM %s.transactions 
                WHERE hash = decode($1, 'hex')`, chainName), hash[2:]) // Strip "0x"
			if err != nil {
				c.JSON(404, gin.H{"error": "Transaction not found"})
				return
			}
			tx.Hash = common.Hex2Bytes(common.Bytes2Hex(tx.Hash))
			c.JSON(200, tx)
		})

		// Latest blocks
		r.GET("/"+chainName+"/latest", func(c *gin.Context) {
			limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
			var blocks []struct {
				ID        int64  `db:"id" json:"id"`
				Hash      []byte `db:"hash" json:"hash"`
				Timestamp int64  `db:"timestamp" json:"timestamp"`
				TxCount   int    `db:"tx_count" json:"tx_count"`
			}
			err := db.DB.Select(&blocks, fmt.Sprintf(`
                SELECT id, hash, timestamp, tx_count 
                FROM %s.blocks 
                ORDER BY id DESC 
                LIMIT $1`, chainName), limit)
			if err != nil {
				c.JSON(500, gin.H{"error": "Failed to fetch latest blocks"})
				return
			}
			for i := range blocks {
				blocks[i].Hash = common.Hex2Bytes(common.Bytes2Hex(blocks[i].Hash))
			}
			c.JSON(200, blocks)
		})

		// Block by number or hash
		r.GET("/"+chainName+"/blocks/:id", func(c *gin.Context) {
			id := c.Param("id")
			var block struct {
				ID         int64  `db:"id" json:"id"`
				Hash       []byte `db:"hash" json:"hash"`
				ParentHash []byte `db:"parent_hash" json:"parent_hash"`
				Timestamp  int64  `db:"timestamp" json:"timestamp"`
				Miner      string `db:"miner" json:"miner"`
				GasLimit   int64  `db:"gas_limit" json:"gas_limit"`
				GasUsed    int64  `db:"gas_used" json:"gas_used"`
				TxCount    int    `db:"tx_count" json:"tx_count"`
			}
			query := fmt.Sprintf(`
                SELECT id, hash, parent_hash, timestamp, miner, gas_limit, gas_used, tx_count 
                FROM %s.blocks 
                WHERE id = $1 OR hash = decode($2, 'hex')`, chainName)
			err := db.DB.Get(&block, query, id, id[2:])
			if err != nil {
				c.JSON(404, gin.H{"error": "Block not found"})
				return
			}
			block.Hash = common.Hex2Bytes(common.Bytes2Hex(block.Hash))
			block.ParentHash = common.Hex2Bytes(common.Bytes2Hex(block.ParentHash))
			c.JSON(200, block)
		})

		// Address details
		r.GET("/"+chainName+"/addresses/:address", func(c *gin.Context) {
			address := c.Param("address")
			var addr struct {
				Address          string `db:"address" json:"address"`
				Balance          string `db:"balance" json:"balance"`
				IsContract       bool   `db:"is_contract" json:"is_contract"`
				LastUpdatedBlock int64  `db:"last_updated_block" json:"last_updated_block"`
			}
			err := db.DB.Get(&addr, fmt.Sprintf(`
                SELECT address, balance, is_contract, last_updated_block 
                FROM %s.addresses 
                WHERE address = $1`, chainName), address)
			if err != nil {
				c.JSON(404, gin.H{"error": "Address not found"})
				return
			}
			c.JSON(200, addr)
		})

		// Search (block, tx, address)
		r.GET("/"+chainName+"/search", func(c *gin.Context) {
			query := c.Query("q")
			if len(query) < 2 {
				c.JSON(400, gin.H{"error": "Query too short"})
				return
			}
			// Try block
			var block struct {
				ID int64 `db:"id" json:"id"`
			}
			err := db.DB.Get(&block, fmt.Sprintf(`
                SELECT id FROM %s.blocks 
                WHERE id = $1 OR hash = decode($2, 'hex')`, chainName), query, query[2:])
			if err == nil {
				c.JSON(200, gin.H{"type": "block", "id": block.ID})
				return
			}
			// Try tx
			var tx struct {
				Hash []byte `db:"hash" json:"hash"`
			}
			err = db.DB.Get(&tx, fmt.Sprintf(`
                SELECT hash FROM %s.transactions 
                WHERE hash = decode($1, 'hex')`, chainName), query[2:])
			if err == nil {
				c.JSON(200, gin.H{"type": "transaction", "hash": "0x" + common.Bytes2Hex(tx.Hash)})
				return
			}
			// Try address
			var addr struct {
				Address string `db:"address" json:"address"`
			}
			err = db.DB.Get(&addr, fmt.Sprintf(`
                SELECT address FROM %s.addresses 
                WHERE address = $1`, chainName), query)
			if err == nil {
				c.JSON(200, gin.H{"type": "address", "address": addr.Address})
				return
			}
			c.JSON(404, gin.H{"error": "Not found"})
		})

		// WebSocket for live updates
		r.GET("/"+chainName+"/live", func(c *gin.Context) {
			conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
			if err != nil {
				c.JSON(500, gin.H{"error": "Failed to upgrade to WebSocket"})
				return
			}
			defer conn.Close()

			conns := wsConns[chainName]
			conns[conn] = true
			wsConns[chainName] = conns

			// Keep connection alive
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					delete(conns, conn)
					break
				}
			}
		})
	}

	log.Info().Msg("Starting API server on :9876")
	r.Run(":9876")
}
