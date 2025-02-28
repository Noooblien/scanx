package indexer

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Fetcher struct {
	client *ethclient.Client
}

func NewFetcher(rpcURL string) (*Fetcher, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err // Return error instead of panicking
	}
	return &Fetcher{client: client}, nil
}

func (f *Fetcher) GetBlock(number int64) (*types.Block, error) {
	return f.client.BlockByNumber(context.Background(), big.NewInt(number))
}

func (f *Fetcher) GetBalance(address string, blockNum *big.Int) (*big.Int, error) {
	addr := common.HexToAddress(address)
	return f.client.BalanceAt(context.Background(), addr, blockNum)
}

func (f *Fetcher) GetCode(address string, blockNum *big.Int) ([]byte, error) {
	addr := common.HexToAddress(address)
	return f.client.CodeAt(context.Background(), addr, blockNum)
}
