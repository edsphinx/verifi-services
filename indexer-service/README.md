# VeriFi Event Indexer Service

Real-time blockchain event indexer that monitors Aptos smart contract events and automatically records activities in the database.

## Features

- **Real-time Event Monitoring**: Polls Aptos RPC every 5 seconds for new transactions
- **Automatic Activity Recording**: Automatically inserts BUY/SELL activities into database
- **Event-driven Architecture**: Registers custom handlers for different event types
- **Progress Tracking**: Maintains sync state to resume from last processed version
- **HTTP API**: Health checks and status endpoints
- **Systemd Integration**: Runs as a system service on VPS

## Architecture

### Event Flow

```
Aptos Blockchain → Event Listener (Polling) → Event Handlers → PostgreSQL Database
                                                                      ↓
                                                              Activity Table
                                                              sync_state Table
```

### Supported Events

1. **SharesMintedEvent** - Records BUY activities
2. **SharesBurnedEvent** - Records SELL activities
3. **MarketCreatedEvent** - Logs new market creation
4. **MarketResolvedEvent** - Updates market status to resolved

## Prerequisites

- Go 1.22+
- PostgreSQL database (shared with main VeriFi project)
- Access to Aptos RPC endpoint (testnet/mainnet)

## Configuration

Environment variables (loaded from parent `.env` or `.env.local`):

```bash
# Database (same as main project)
DATABASE_URL=postgresql://user:password@host:5432/database

# Aptos Network
NEXT_PUBLIC_APTOS_NETWORK=testnet
NEXT_PUBLIC_PUBLISHER_ACCOUNT_ADDRESS=0x...

# Service Port (optional, defaults to 3002)
INDEXER_PORT=3002
```

## Local Development

### Install Dependencies

```bash
go mod download
```

### Run Locally

```bash
# From indexer-service directory
go run cmd/server/main.go
```

The service will:
1. Connect to the database
2. Run migrations (create sync_state table)
3. Start HTTP server on port 3002
4. Begin polling Aptos blockchain for events

### API Endpoints

- `GET /health` - Health check
- `GET /status` - Current indexing status and last processed version

## Deployment

### Deploy to VPS

The service is designed to run on a VPS alongside the sync-service.

**VPS Configuration** (stored in main `.env`):
```bash
VERIFI_VPS_ADDRESS=198.144.183.32
VERIFI_VPS_ROOT_PASS=<your_password>
```

Deploy command:
```bash
# From indexer-service directory
./scripts/deploy.sh
```

This will:
1. Build the Go binary for Linux
2. Create deployment package
3. Upload to VPS using credentials from `.env`
4. Install as systemd service
5. Start the service automatically

### Systemd Service Management

On the VPS:

```bash
# Check status
sudo systemctl status verifi-indexer

# View logs
sudo journalctl -u verifi-indexer -f

# Restart service
sudo systemctl restart verifi-indexer

# Stop service
sudo systemctl stop verifi-indexer
```

### Docker Deployment (Alternative)

```bash
# Build image
docker-compose build

# Run service
docker-compose up -d

# View logs
docker-compose logs -f
```

## How It Works

### Event Listener

The `EventListener` continuously polls the Aptos blockchain:

1. **Get Latest Ledger Version**: Queries Aptos RPC for current blockchain version
2. **Fetch Transactions**: Retrieves transactions in batches (100 per batch)
3. **Filter Events**: Looks for events from the VeriFi module
4. **Process Events**: Executes registered handlers for each event type
5. **Update Progress**: Saves last processed version to database

### Event Handlers

Each event type has a dedicated handler:

```go
// Example: SharesMintedEvent handler
func (l *EventListener) handleSharesMinted(ctx context.Context, event Event, tx TransactionEvent) error {
    // Extract event data
    marketAddress := event.Data["market_address"]
    user := event.Data["user"]
    aptAmount := event.Data["apt_amount_in"]
    shares := event.Data["shares_out"]
    isYes := event.Data["is_yes"]

    // Insert into Activity table
    // ...
}
```

### Database Schema

The indexer maintains a `sync_state` table to track progress:

```sql
CREATE TABLE sync_state (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);
```

Activities are recorded in the existing `Activity` table from the main project.

## Monitoring

### Health Check

```bash
curl http://198.144.183.32:3002/health
```

Response:
```json
{
  "status": "healthy",
  "service": "verifi-indexer-service",
  "time": 1234567890
}
```

### Status Check

```bash
curl http://198.144.183.32:3002/status
```

Response:
```json
{
  "status": "running",
  "last_version": 123456789,
  "network": "testnet"
}
```

## Architecture Integration

This service works alongside the main VeriFi protocol:

```
┌─────────────────────┐
│  Next.js Frontend   │
│  (verifi-protocol)  │
└──────────┬──────────┘
           │
           ↓
┌─────────────────────┐       ┌──────────────────────┐
│  PostgreSQL DB      │←──────│  Sync Service        │
│  (Supabase)         │       │  (Metrics/Stats)     │
└──────────┬──────────┘       │  Port: 3001          │
           ↑                  └──────────────────────┘
           │
┌──────────┴──────────┐
│  Indexer Service    │
│  (Event Listener)   │
│  Port: 3002         │
└──────────┬──────────┘
           ↓
┌─────────────────────┐
│  Aptos Blockchain   │
│  (Smart Contracts)  │
└─────────────────────┘
```

### Dual Microservice Architecture

The VeriFi backend consists of two Go microservices:

1. **sync-service** (Port 3001)
   - Scheduled metrics synchronization
   - Pool stats updates
   - Volume calculations
   - Cron-based (hourly/15min/5min)

2. **indexer-service** (Port 3002)
   - Real-time event monitoring
   - Activity recording
   - Blockchain polling (5 second interval)

Both services:
- Share the same PostgreSQL database
- Run on the same VPS
- Use systemd for process management
- Load config from main project .env file

## Troubleshooting

### Service Not Starting

Check logs:
```bash
sudo journalctl -u verifi-indexer -n 50
```

Common issues:
- Database connection failed: Verify DATABASE_URL in .env
- Module address not set: Check NEXT_PUBLIC_PUBLISHER_ACCOUNT_ADDRESS
- Port already in use: Change INDEXER_PORT

### Events Not Being Processed

1. Check if service is running: `sudo systemctl status verifi-indexer`
2. Verify network connectivity to Aptos RPC
3. Check last processed version: `curl http://localhost:3002/status`
4. Review event handlers in listener.go

### Database Connection Issues

Ensure the DATABASE_URL is accessible from VPS:
```bash
# Test connection
psql "$DATABASE_URL"
```

## Performance

- **Polling Interval**: 5 seconds (configurable in listener.go)
- **Batch Size**: 100 transactions per request
- **Memory Usage**: ~20-50 MB
- **CPU Usage**: Minimal (~1-5%)

## Security Considerations

- Service runs as root (consider creating dedicated user)
- Database credentials stored in .env file
- No authentication on HTTP endpoints (add if exposing publicly)
- Use firewall to restrict port 3002 access

## Future Enhancements

- [ ] WebSocket support for real-time updates
- [ ] Webhook notifications on specific events
- [ ] Prometheus metrics export
- [ ] GraphQL API for event queries
- [ ] Event replay functionality
- [ ] Multi-module support (index multiple contracts)

## License

Part of the VeriFi Protocol project.
