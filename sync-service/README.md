# VeriFi Sync Service

Go microservice for automated blockchain data synchronization running on VPS.

## Features

- ⏰ **Automated Cron Jobs**
  - Metrics Sync: Every hour
  - Pools Sync: Every 15 minutes
  - Activities Sync: Every 5 minutes

- 🔌 **HTTP API**
  - Manual sync triggers
  - Health checks
  - Service statistics

- 📊 **Metrics Calculation**
  - volume24h, volume7d, totalVolume
  - Unique traders count
  - Pool reserves and LP positions

- 🚀 **Performance**
  - Low memory footprint (~10MB)
  - Fast startup time (<1s)
  - Concurrent processing with goroutines

## Quick Start

### Local Development

```bash
# Install dependencies
go mod download

# Copy environment file
cp .env.example .env
# Edit .env with your DATABASE_URL

# Run service
go run cmd/server/main.go
```

### VPS Deployment

```bash
# Make deploy script executable
chmod +x scripts/deploy.sh

# Deploy to VPS
./scripts/deploy.sh 198.144.183.32 root
```

### Docker Deployment

```bash
# Build image
docker build -t verifi-sync-service .

# Run with docker-compose
docker-compose up -d

# View logs
docker-compose logs -f
```

## API Endpoints

### Health Check
```bash
GET http://your-vps:3001/health
```

### Manual Sync Triggers
```bash
# Sync metrics (volume, traders)
POST http://your-vps:3001/sync/metrics

# Sync pools (reserves, LP)
POST http://your-vps:3001/sync/pools

# Sync activities (backup)
POST http://your-vps:3001/sync/activities
```

### Service Statistics
```bash
GET http://your-vps:3001/status
```

Response:
```json
{
  "lastMetricsSync": "2025-10-04T22:30:00Z",
  "lastPoolsSync": "2025-10-04T22:15:00Z",
  "lastActivitiesSync": "2025-10-04T22:25:00Z",
  "metricsSyncCount": 24,
  "poolsSyncCount": 96,
  "activitiesSyncCount": 288,
  "errors": 0
}
```

## Cron Schedule

| Job | Schedule | Description |
|-----|----------|-------------|
| Metrics Sync | `0 0 * * * *` | Every hour at :00 |
| Pools Sync | `0 */15 * * * *` | Every 15 minutes |
| Activities Sync | `0 */5 * * * *` | Every 5 minutes |

## Environment Variables

```bash
# Required
DATABASE_URL=postgresql://user:pass@host:5432/db?sslmode=require

# Optional
PORT=3001                    # Default: 3001
ENVIRONMENT=production       # Default: development
```

## Systemd Service

The deploy script automatically creates a systemd service:

```bash
# Check status
sudo systemctl status verifi-sync

# View logs
sudo journalctl -u verifi-sync -f

# Restart service
sudo systemctl restart verifi-sync

# Stop service
sudo systemctl stop verifi-sync
```

## Monitoring

### Check if service is running
```bash
curl http://198.144.183.32:3001/health
```

### View real-time statistics
```bash
watch -n 5 'curl -s http://198.144.183.32:3001/status | jq'
```

### Monitor logs
```bash
ssh root@198.144.183.32 'journalctl -u verifi-sync -f'
```

## Architecture

```
┌─────────────────────────────────────┐
│   VPS (198.144.183.32)              │
├─────────────────────────────────────┤
│   VeriFi Sync Service (Go)          │
│   • Port: 3001                      │
│   • Memory: ~10MB                   │
│   • CPU: <1%                        │
└─────────────────────────────────────┘
           ↓ SQL Queries
┌─────────────────────────────────────┐
│   Supabase PostgreSQL               │
│   • Markets Table                   │
│   • Activities Table                │
│   • Pools Table                     │
└─────────────────────────────────────┘
```

## Development

### Project Structure
```
sync-service/
├── cmd/
│   └── server/
│       └── main.go           # Entry point
├── internal/
│   ├── config/
│   │   └── config.go         # Configuration
│   ├── db/
│   │   └── db.go             # Database client
│   └── sync/
│       └── service.go        # Sync logic
├── scripts/
│   └── deploy.sh             # Deployment script
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

### Adding New Sync Jobs

1. Add function to `internal/sync/service.go`
2. Register cron job in `cmd/server/main.go`
3. Add HTTP endpoint for manual trigger

Example:
```go
// In service.go
func (s *Service) SyncNewFeature(ctx context.Context) error {
    // Your sync logic here
    return nil
}

// In main.go
cronScheduler.AddFunc("0 */10 * * * *", func() {
    if err := syncService.SyncNewFeature(context.Background()); err != nil {
        log.Error().Err(err).Msg("Sync failed")
    }
})
```

## Troubleshooting

### Service won't start
```bash
# Check logs
journalctl -u verifi-sync -n 50

# Check if port is in use
netstat -tulpn | grep 3001

# Verify DATABASE_URL
cat /opt/verifi-sync-service/.env
```

### High CPU usage
```bash
# Check stats endpoint
curl http://localhost:3001/status

# Reduce sync frequency in main.go
```

### Database connection errors
```bash
# Test connection
psql $DATABASE_URL -c "SELECT 1"

# Verify SSL mode
# Add ?sslmode=require to DATABASE_URL
```

## Security Notes

⚠️ **IMPORTANT**: Never commit `.env` files or credentials to git!

- Store credentials in VPS environment only
- Use firewall to restrict port 3001 access
- Consider using SSH tunneling for API access
- Rotate database credentials regularly

## License

Proprietary - VeriFi Protocol
