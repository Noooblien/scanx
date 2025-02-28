package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/Noooblien/scanx/internal/db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/rs/zerolog/log"
)

type Indexer struct {
	fetcher          *Fetcher
	client           *ethclient.Client
	rpc              *rpc.Client
	historicalRpcSem chan struct{}
	newRpcSem        chan struct{}
	chainName        string
	chainID          int64
	blocksProcessed  int
}

type TraceCall struct {
	From         common.Address `json:"from"`
	To           common.Address `json:"to"`
	Value        *big.Int       `json:"value"`
	Gas          uint64         `json:"gas"`
	GasUsed      uint64         `json:"gasUsed"`
	Input        string         `json:"input"`
	Output       string         `json:"output"`
	Type         string         `json:"type"`
	TraceAddress []int          `json:"traceAddress"`
}

type TraceResult struct {
	Calls []TraceCall `json:"calls"`
}

func NewIndexer(rpcURL, chainName string, chainID int64) (*Indexer, error) {
	fetcher, err := NewFetcher(rpcURL)
	if err != nil {
		return nil, err
	}
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	rpcClient, err := rpc.DialContext(context.Background(), rpcURL)
	if err != nil {
		return nil, err
	}
	db.InitChainSchema(chainName)
	return &Indexer{
		fetcher:          fetcher,
		client:           client,
		rpc:              rpcClient,
		historicalRpcSem: make(chan struct{}, 50),
		newRpcSem:        make(chan struct{}, 20),
		chainName:        chainName,
		chainID:          chainID,
	}, nil
}

func (i *Indexer) Start(startBlock int64) {
	log.Info().Str("chain", i.chainName).Int64("start_block", startBlock).Msg("Starting indexer")
	startTime := time.Now()
	i.historicalRpcSem <- struct{}{}
	latestUint64, err := i.fetcher.client.BlockNumber(context.Background())
	<-i.historicalRpcSem
	if err != nil {
		log.Fatal().Err(err).Str("chain", i.chainName).Msg("Failed to get latest block number")
	}
	latest := int64(latestUint64)
	log.Info().Str("chain", i.chainName).Int64("latest_block", latest).Msg("Fetched latest block number")

	lastIndexed := db.GetLastIndexedBlock(i.chainName)
	if lastIndexed < startBlock || lastIndexed > latest {
		lastIndexed = latest // Start from latest if DB is fresh or invalid
		log.Info().Str("chain", i.chainName).Int64("last_indexed", lastIndexed).Msg("Starting from latest block due to fresh or invalid DB")
	} else {
		log.Info().Str("chain", i.chainName).Int64("last_indexed", lastIndexed).Msg("Resuming from last indexed block")
	}

	var wg sync.WaitGroup

	// Historical sync (latest to startBlock, reverse)
	wg.Add(1)
	go func() {
		defer wg.Done()
		workers := 30 // can increase
		blockChan := make(chan int64, 5000)
		var workerWg sync.WaitGroup

		for w := 0; w < workers; w++ {
			workerWg.Add(1)
			go func(workerID int) {
				defer workerWg.Done()
				for num := range blockChan {
					i.historicalRpcSem <- struct{}{}
					var exists bool
					err := db.DB.Get(&exists, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s.blocks WHERE id = $1)", i.chainName), num)
					if err != nil {
						log.Error().Err(err).Str("chain", i.chainName).Int64("block", num).Msg("Failed to check block existence")
						<-i.historicalRpcSem
						continue
					}
					if exists {
						<-i.historicalRpcSem
						continue
					}
					var block *types.Block
					for attempt := 0; attempt < 3; attempt++ {
						block, err = i.fetcher.GetBlock(num)
						if err == nil {
							break
						}
						log.Warn().Err(err).Str("chain", i.chainName).Int64("block", num).Int("attempt", attempt+1).Msg("Retrying block fetch")
						time.Sleep(time.Second * time.Duration(attempt+1))
					}
					<-i.historicalRpcSem
					if err != nil {
						log.Error().Err(err).Str("chain", i.chainName).Int64("block", num).Msg("Failed to fetch block after retries")
						continue
					}
					i.processBlock(block, false)
					i.blocksProcessed++
					db.SetLastIndexedBlock(i.chainName, num)
					if i.blocksProcessed%100 == 0 {
						log.Info().
							Str("chain", i.chainName).
							Int64("block", num).
							Int("tx_count", len(block.Transactions())).
							Msg("Processed historical block")
					}
				}
				log.Info().Str("chain", i.chainName).Int("worker", workerID).Msg("Historical worker completed")
			}(w)
		}

		expectedBlocks := latest - startBlock
		log.Info().Str("chain", i.chainName).Int64("expected_blocks", expectedBlocks).Msg("Starting historical block queue (reverse)")
		for num := latest - 1; num >= startBlock; num-- { // Reverse from latest-1 to startBlock
			blockChan <- num
		}
		close(blockChan)
		workerWg.Wait()
		elapsed := time.Since(startTime).Seconds()
		log.Info().
			Str("chain", i.chainName).
			Int("blocks_processed", i.blocksProcessed).
			Float64("total_time_sec", elapsed).
			Msg("Historical sync completed")
	}()

	// New block sync (forward from latest)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		lastIndexedNew := latest // Start new sync from initial latest
		log.Info().Str("chain", i.chainName).Int64("last_indexed_new", lastIndexedNew).Msg("Starting new block sync")

		for range ticker.C {
			i.newRpcSem <- struct{}{}
			latestUint64, err := i.fetcher.client.BlockNumber(context.Background())
			<-i.newRpcSem
			if err != nil {
				log.Warn().Err(err).Str("chain", i.chainName).Msg("Failed to get latest block number")
				continue
			}
			latest = int64(latestUint64)
			log.Info().Str("chain", i.chainName).Int64("latest", latest).Int64("last_indexed_new", lastIndexedNew).Msg("Checking for new blocks")

			if latest > lastIndexedNew {
				log.Info().Str("chain", i.chainName).Int64("new_blocks", latest-lastIndexedNew).Msg("Found new blocks to process")
				var totalGasUsed, totalTxs int64
				for num := lastIndexedNew + 1; num <= latest; num++ {
					i.newRpcSem <- struct{}{}
					var exists bool
					err := db.DB.Get(&exists, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s.blocks WHERE id = $1)", i.chainName), num)
					if err != nil {
						log.Error().Err(err).Str("chain", i.chainName).Int64("block", num).Msg("Failed to check block existence")
						<-i.newRpcSem
						continue
					}
					if exists {
						log.Debug().Str("chain", i.chainName).Int64("block", num).Msg("Block already exists")
						<-i.newRpcSem
						continue
					}
					var block *types.Block
					for attempt := 0; attempt < 3; attempt++ {
						block, err = i.fetcher.GetBlock(num)
						if err == nil {
							break
						}
						log.Warn().Err(err).Str("chain", i.chainName).Int64("block", num).Int("attempt", attempt+1).Msg("Retrying block fetch")
						time.Sleep(time.Second * time.Duration(attempt+1))
					}
					<-i.newRpcSem
					if err != nil {
						log.Error().Err(err).Str("chain", i.chainName).Int64("block", num).Msg("Failed to fetch new block after retries")
						continue
					}
					i.processBlock(block, true)
					i.blocksProcessed++
					totalGasUsed += int64(block.GasUsed())
					totalTxs += int64(len(block.Transactions()))
					db.SetLastIndexedBlock(i.chainName, num)
					log.Info().
						Str("chain", i.chainName).
						Int64("block", num).
						Int("tx_count", len(block.Transactions())).
						Msg("Indexed new block")
				}
				elapsed := time.Since(startTime).Seconds()
				var avgTps float64
				var avgGasPerTx int64
				if totalTxs > 0 {
					avgTps = float64(totalTxs) / elapsed
					avgGasPerTx = totalGasUsed / totalTxs
				}
				db.DB.Exec(fmt.Sprintf(`
                    INSERT INTO %s.stats (timestamp, total_blocks, total_txs, total_gas_used, avg_tps, avg_gas_per_tx)
                    VALUES ($1, $2, $3, $4, $5, $6)`, i.chainName),
					time.Now().Unix(), latest, totalTxs, totalGasUsed, avgTps, avgGasPerTx,
				)
				lastIndexedNew = latest
				log.Info().Str("chain", i.chainName).Int64("updated_last_indexed_new", lastIndexedNew).Msg("Updated last indexed block")
			}
		}
	}()

	wg.Wait()
	log.Info().Str("chain", i.chainName).Msg("Indexer stopped")
}

func (i *Indexer) processBlock(block *types.Block, updateBalances bool) {
	baseFee := "0"
	if block.BaseFee() != nil {
		baseFee = block.BaseFee().String()
	}
	db.DB.MustExec(fmt.Sprintf(`
        INSERT INTO %s.blocks (
            id, hash, parent_hash, timestamp, miner, gas_limit, gas_used, base_fee_per_gas,
            difficulty, total_difficulty, transactions_root, receipts_root, state_root,
            size, nonce, logs_bloom, extra_data, tx_count
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
        ON CONFLICT (id) DO NOTHING`, i.chainName),
		block.Number().Int64(), block.Hash().Bytes(), block.ParentHash().Bytes(), block.Time(),
		block.Coinbase().Hex(), block.GasLimit(), block.GasUsed(), baseFee, block.Difficulty().String(),
		"0", block.TxHash().Bytes(), block.ReceiptHash().Bytes(), block.Root().Bytes(),
		block.Size(), block.Nonce(), block.Bloom().Bytes(), block.Extra(), len(block.Transactions()),
	)

	for _, tx := range block.Transactions() {
		i.processTransaction(tx, block, updateBalances)
	}
}

func (i *Indexer) processTransaction(tx *types.Transaction, block *types.Block, updateBalances bool) {
	receipt, err := i.fetcher.client.TransactionReceipt(context.Background(), tx.Hash())
	if err != nil {
		log.Error().Err(err).
			Str("chain", i.chainName).
			Str("tx_hash", tx.Hash().Hex()).
			Int64("block", block.Number().Int64()).
			Msg("Failed to get receipt")
		return
	}

	from, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		log.Error().Err(err).
			Str("chain", i.chainName).
			Str("tx_hash", tx.Hash().Hex()).
			Int64("block", block.Number().Int64()).
			Msg("Failed to get sender")
		return
	}
	to := ""
	if tx.To() != nil {
		to = tx.To().Hex()
	}

	logsJSON, _ := json.Marshal(receipt.Logs)
	maxFee := "0"
	maxPriorityFee := "0"
	if tx.Type() == types.DynamicFeeTxType {
		maxFee = tx.GasFeeCap().String()
		maxPriorityFee = tx.GasTipCap().String()
	}

	v, r, s := tx.RawSignatureValues()

	db.DB.MustExec(fmt.Sprintf(`
        INSERT INTO %s.transactions (
            hash, block_number, block_hash, "from", "to", value, gas, gas_used, gas_price,
            max_fee_per_gas, max_priority_fee_per_gas, effective_gas_price, nonce, input,
            type, transaction_index, chain_id, v, r, s, timestamp, logs_bloom, logs, status,
            contract_address, token_transfer
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
        ON CONFLICT (hash) DO NOTHING`, i.chainName),
		tx.Hash().Bytes(), block.Number().Int64(), block.Hash().Bytes(), from.Hex(), to,
		tx.Value().String(), tx.Gas(), receipt.GasUsed, tx.GasPrice().String(), maxFee,
		maxPriorityFee, receipt.EffectiveGasPrice.String(), tx.Nonce(), tx.Data(),
		tx.Type(), receipt.TransactionIndex, i.chainID, v.Bytes(),
		r.Bytes(), s.Bytes(), block.Time(), receipt.Bloom.Bytes(), logsJSON,
		receipt.Status == 1, receipt.ContractAddress.Hex(), i.detectTokenTransfer(receipt),
	)
	log.Info().
		Str("chain", i.chainName).
		Str("tx_hash", tx.Hash().Hex()).
		Int64("block", block.Number().Int64()).
		Msg("Processed transaction")

	var traceResult TraceResult
	err = i.rpc.CallContext(context.Background(), &traceResult, "debug_traceTransaction", tx.Hash().Hex(), nil)
	if err == nil && len(traceResult.Calls) > 0 {
		for _, call := range traceResult.Calls {
			db.DB.MustExec(fmt.Sprintf(`
                INSERT INTO %s.internal_transactions (
                    tx_hash, "from", "to", value, call_type, gas, gas_used, input, output, trace_address
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
                ON CONFLICT DO NOTHING`, i.chainName),
				tx.Hash().Bytes(), call.From.Hex(), call.To.Hex(), call.Value.String(),
				call.Type, call.Gas, call.GasUsed, common.Hex2Bytes(call.Input[2:]),
				common.Hex2Bytes(call.Output[2:]), call.TraceAddress,
			)
			log.Info().
				Str("chain", i.chainName).
				Str("tx_hash", tx.Hash().Hex()).
				Str("from", call.From.Hex()).
				Str("to", call.To.Hex()).
				Msg("Processed internal transaction")
		}
	} else if err != nil {
		log.Info().
			Str("chain", i.chainName).
			Str("tx_hash", tx.Hash().Hex()).
			Err(err).
			Msg("Skipping internal tx tracing")
	}

	if receipt.ContractAddress != (common.Address{}) {
		i.trackContract(receipt.ContractAddress.Hex(), from.Hex(), block.Number(), tx.Hash(), updateBalances)
	}

	if updateBalances {
		i.updateAccount(from.Hex(), block.Number())
		if to != "" {
			i.updateAccount(to, block.Number())
		}
	} else {
		i.trackAddress(from.Hex(), block.Number())
		if to != "" {
			i.trackAddress(to, block.Number())
		}
	}
}

func (i *Indexer) detectTokenTransfer(receipt *types.Receipt) bool {
	for _, log := range receipt.Logs {
		if len(log.Topics) == 3 && log.Topics[0].Hex() == "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef" {
			return true
		}
	}
	return false
}

func (i *Indexer) updateAccount(address string, blockNum *big.Int) {
	balance, err := i.fetcher.GetBalance(address, blockNum)
	if err != nil {
		log.Warn().Err(err).
			Str("chain", i.chainName).
			Str("address", address).
			Int64("block", blockNum.Int64()).
			Msg("Failed to get balance, using 0")
		balance = big.NewInt(0)
	}

	code, err := i.fetcher.GetCode(address, blockNum)
	if err != nil {
		log.Warn().Err(err).
			Str("chain", i.chainName).
			Str("address", address).
			Int64("block", blockNum.Int64()).
			Msg("Failed to get code, assuming not a contract")
		code = nil
	}

	db.DB.MustExec(fmt.Sprintf(`
        INSERT INTO %s.addresses (address, balance, is_contract, bytecode, last_updated_block)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (address) DO UPDATE SET balance = $2, is_contract = $3, bytecode = $4, last_updated_block = $5`, i.chainName),
		address, balance.String(), len(code) > 0, code, blockNum.Int64(),
	)
}

func (i *Indexer) trackAddress(address string, blockNum *big.Int) {
	db.DB.MustExec(fmt.Sprintf(`
        INSERT INTO %s.addresses (address, balance, is_contract, bytecode, last_updated_block)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (address) DO UPDATE SET last_updated_block = $5`, i.chainName),
		address, "0", false, nil, blockNum.Int64(),
	)
}

func (i *Indexer) trackContract(address, creator string, blockNum *big.Int, txHash common.Hash, fetchBytecode bool) {
	var bytecode []byte
	if fetchBytecode {
		var err error
		bytecode, err = i.fetcher.GetCode(address, blockNum)
		if err != nil {
			log.Warn().Err(err).
				Str("chain", i.chainName).
				Str("address", address).
				Int64("block", blockNum.Int64()).
				Msg("Failed to get bytecode")
			bytecode = nil
		}
	}

	db.DB.MustExec(fmt.Sprintf(`
        INSERT INTO %s.contracts (address, creator, creation_block, creation_tx_hash, bytecode)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (address) DO NOTHING`, i.chainName),
		address, creator, blockNum.Int64(), txHash.Bytes(), bytecode,
	)
}
