# VeriFi Services

Microservices for blockchain indexing and metrics synchronization on Aptos.

## Services

### 1. Indexer Service (Port 3002)

Real-time blockchain event monitoring and indexing.

**Features:**
- Event polling (5-second intervals)
- Market event tracking (Created/Resolved)
- Activity recording (BUY/SELL/SWAP)
- API key rotation (8 keys round-robin)
- Progress tracking (sync_state table)

**Endpoints:**
- `GET /health` - Health check
- `GET /status` - Indexer status and progress

### 2. Sync Service (Port 3001)

Periodic metrics synchronization and aggregation.

**Features:**
- Metrics sync (hourly)
- Pool stats updates (15 min)
- Activity aggregation (5 min)
- HTTP API for manual triggers

**Endpoints:**
- `GET /health` - Health check
- `GET /status` - Service status
- `POST /sync/metrics` - Trigger metrics sync
- `POST /sync/pools` - Trigger pool stats sync
- `POST /sync/activities` - Trigger activity sync

## Architecture

```
Aptos Blockchain
       ↓
Indexer Service (Port 3002) → Database ← Sync Service (Port 3001)
       ↓                                        ↓
Event Processing                         Metrics Calculation
API Key Rotation                         Cron Jobs
```

## Setup

1. Install Go dependencies:
```bash
cd indexer-service && go mod download
cd ../sync-service && go mod download
```

2. Configure environment variables:
```bash
# Database
DATABASE_URL=postgresql://user:password@host:5432/database

# API Keys (comma-separated, NO spaces)
APTOS_API_KEYS=key1,key2,key3,key4
NODIT_API_KEYS=nodit1,nodit2,nodit3,nodit4

# Network
APTOS_NETWORK=testnet
MODULE_ADDRESS=0x...
```

3. Run locally:
```bash
# Indexer Service
cd indexer-service
go run cmd/server/main.go

# Sync Service (separate terminal)
cd sync-service
go run cmd/server/main.go
```

## Deployment (VPS)

Both services include deployment scripts for Ubuntu VPS:

```bash
# Deploy Indexer Service
cd indexer-service
./scripts/deploy.sh

# Deploy Sync Service
cd sync-service
./scripts/deploy.sh
```

Services run as systemd units:
- `verifi-indexer.service`
- `verifi-sync.service`

**Management:**
```bash
# Check status
sudo systemctl status verifi-indexer
sudo systemctl status verifi-sync

# View logs
sudo journalctl -u verifi-indexer -f
sudo journalctl -u verifi-sync -f

# Restart
sudo systemctl restart verifi-indexer
sudo systemctl restart verifi-sync
```

## API Key Rotation

The indexer service rotates between 8 API keys (4 Aptos + 4 Nodit) to avoid rate limits:

- Round-robin distribution
- 100ms minimum delay between same key uses
- Automatic throttling
- Stats endpoint for monitoring

## Database Schema

**Required tables:**
- `markets` - Market data
- `activities` - User activities
- `tapp_pools` - AMM pools
- `sync_state` - Indexer progress tracking

## Monitoring

Check service health:
```bash
# Indexer Service
curl http://VPS_IP:3002/health
curl http://VPS_IP:3002/status

# Sync Service
curl http://VPS_IP:3001/health
curl http://VPS_IP:3001/status
```

## Tech Stack

- **Language**: Go 1.21+
- **Database**: PostgreSQL (via Supabase)
- **ORM**: GORM
- **Scheduler**: robfig/cron
- **HTTP**: Gin framework

## License

MIT
