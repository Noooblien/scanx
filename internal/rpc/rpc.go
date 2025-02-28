package rpc

import (
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type RPCClient struct {
	Client *rpc.Client
}

func NewRPCClient(rpcURL string) *RPCClient {
	client, err := rpc.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to RPC: %v", err)
	}
	return &RPCClient{Client: client}
}

func (r *RPCClient) GetLatestBlockNumber() uint64 {
	var latestBlock big.Int
	err := r.Client.Call(&latestBlock, "eth_blockNumber")
	if err != nil {
		log.Fatalf("Failed to get latest block: %v", err)
	}
	return latestBlock.Uint64()
}

func (r *RPCClient) GetBlockByNumber(blockNumber uint64) (*types.Block, error) {
	var block *types.Block
	err := r.Client.Call(&block, "eth_getBlockByNumber", blockNumber, true)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func (r *RPCClient) GetTransactionReceipt(txHash common.Hash) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := r.Client.Call(&receipt, "eth_getTransactionReceipt", txHash.Hex())
	if err != nil {
		return nil, err
	}
	return receipt, nil
}
