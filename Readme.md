# ScanX - Multi-chain Blockchain Indexer


**ScanX** is a high-performance blockchain indexer designed to process and serve data from multiple chains simultaneously. It syncs historical and real-time block data, powering blockchain explorers with fast APIs and live updates. Built for scale, ScanX handles 30+ chains with millions of transactions each, making blockchain data accessible and snappy.

Current release: **`v0.0.1`**—multi-chain indexer with explorer-ready APIs.

---

## Features

- **Block & Transaction Indexing**: Syncs historical blocks (latest to 0) and new blocks with full transaction details.
- **Contract Tracking**: Indexes smart contract deployments with creator and transaction hash.
- **Internal Transactions**: Supports tracing internal txs (if RPC allows) for deeper insights.
- **Chain Stats**: Tracks total blocks, transactions, gas usage, and transactions per second (TPS).
- **REST API**: Serves data via endpoints like `/blocks`, `/transactions`, `/contracts`, `/stats`.
- **Concurrent Processing**: Uses 100 workers (50/chain) with RPC and DB connection pooling for speed and stability.

---

## Installation

### Prerequisites
- **Go**: 1.21+ (for building from source)
- **PostgreSQL**: 13+ (for data storage)
- **RPC Endpoints**: One per chain (e.g., `http://35.200.235.35/jsonrpc` for `chain_name`)

### Steps
1. **Clone the Repo**
   ```bash
   git clone https://github.com/Noooblien/scanx.git
   cd scanx
   ```

2. **Install Dependencies**
   ```bash
   go mod tidy
   ```

3. **Set Up PostgreSQL**
   Create a database:
   ```sql
   CREATE DATABASE scanx;
   ```

4. **Configure Chains**
   Edit `config.json` with your chains:
   ```json
   {
       "db_conn": "postgres://user:password@localhost:5432/scanx?sslmode=disable",
       "username": "admin",
       "password": "secret",
       "chains": [
           {"name": "chain_name", "rpc_url": "http://your-rpc.com", "chain_id": 98765, "start_block": 0},
           {"name": "chain_name_2", "rpc_url": "http://your.rpc.url", "chain_id": 76557, "start_block": 0}
       ]
   }
   ```

5. **Build and Run**
   ```bash
   go build -ldflags "-X main.Version=v0.0.1" -o ./build/scanx ./cmd/scanx
   ./build/scanx -config config.json
   ```

---

## Usage

### Running the Indexer
Start ScanX to sync blocks and serve APIs:
```bash
./build/scanx -config config.json
```

- **Historical Sync**: Indexes from latest block back to 0 (configurable via `start_block`).
- **New Block Sync**: Indexes new blocks in real-time from the initial latest height.

### API Endpoints
Access via `http://localhost:9876/<chain_name>/endpoint`:
- **Latest Blocks**: `GET /chain_name/latest?limit=10`
- **Transactions**: `GET /chain_name/transactions?page=1&limit=50`
- **Block by ID/Hash**: `GET /chain_name/blocks/8508636`
- **Transaction by Hash**: `GET /chain_name/transactions/0x4516c...`
- **Address Details**: `GET /chain_name/addresses/0x123...`
- **Search**: `GET /chain_name/search?q=8508636`
- **Live Updates**: WebSocket `ws://localhost:9876/chain_name/live`

Example:
```bash
curl http://localhost:9876/chain_name/latest
```

---

## Configuration

- **`config.json`**:
  - `db_conn`: Postgres connection string.
  - `chains`: Array of chain objects with `name`, `rpc_url`, `chain_id`, and `start_block`.

- **Scaling**: 
  - 50 workers/chain by default—handles 30 chains with 1M txns each.
  - Postgres set to 150 connections—bump to 1000+ for 30 chains (`postgresql.conf`: `max_connections=1100`).

---


## Contributing
1. Fork the repo.
2. Create a branch (`git checkout -b feature/your-thing`).
3. Commit changes (`git commit -m "feat: your dope change"`).
4. Push (`git push origin feature/your-thing`).
5. Open a PR to `development`.

---

## License
MIT License—do what you want, just give a shoutout!

---

## Roadmap
- **v0.0.2**: Batching for 10x sync speed.
- **v0.1.0**: Prometheus metrics for blocks/sec and lag.
- **v0.2.0**: Full 30-chain deployment with optimized APIs.

Built by [Noooblien](https://github.com/Noooblien) and the squad—let's make blockchain data fly!

### Squad
[Kritarth Agrawal](https://github.com/kritarth1107)
[ComputerKeeda](https://github.com/ComputerKeeda)
[Deadlium](https://github.com/deadlium)
[Slayer](https://github.com/SlayeR6889)
[Aakash4Dev](https://github.com/aakash4dev)

### Notes
- **Intro**: "What is ScanX?"—straight from the release, sets the vibe.
- **Features**: Matches your list—block/tx indexing, contracts, internal txns, stats, APIs, concurrency.
- **Setup**: Clear steps—clone, build, config, run. Assumes Postgres and RPCs are ready.
- **Usage**: Shows how to run and hit APIs—explorer-ready examples.
- **Roadmap**: Hints at speed (batching), monitoring, and 30-chain scale—keeps it hype.


