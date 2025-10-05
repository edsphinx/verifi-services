#!/bin/bash

# VeriFi Sync Service Deployment Script
# Usage: ./deploy.sh [vps-ip] [ssh-user]

set -e

VPS_IP="${1:-198.144.183.32}"
SSH_USER="${2:-root}"
SERVICE_NAME="verifi-sync-service"
DEPLOY_DIR="/opt/$SERVICE_NAME"

echo "🚀 Deploying VeriFi Sync Service to $SSH_USER@$VPS_IP"

# Build binary locally
echo "📦 Building Go binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o sync-service ./cmd/server

# Create deployment package
echo "📁 Creating deployment package..."
tar -czf deploy.tar.gz sync-service .env.example

# Upload to VPS
echo "⬆️  Uploading to VPS..."
scp deploy.tar.gz $SSH_USER@$VPS_IP:/tmp/

# Deploy on VPS
echo "🔧 Installing on VPS..."
ssh $SSH_USER@$VPS_IP << 'EOF'
    # Create directory
    mkdir -p /opt/verifi-sync-service
    cd /opt/verifi-sync-service

    # Extract files
    tar -xzf /tmp/deploy.tar.gz
    rm /tmp/deploy.tar.gz

    # Set permissions
    chmod +x sync-service

    # Create systemd service
    cat > /etc/systemd/system/verifi-sync.service << 'SERVICE'
[Unit]
Description=VeriFi Sync Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/verifi-sync-service
ExecStart=/opt/verifi-sync-service/sync-service
Restart=always
RestartSec=10
Environment=DATABASE_URL=your_database_url_here

[Install]
WantedBy=multi-user.target
SERVICE

    # Reload systemd and start service
    systemctl daemon-reload
    systemctl enable verifi-sync.service
    systemctl restart verifi-sync.service

    echo "✅ Service deployed and started"
    systemctl status verifi-sync.service --no-pager
EOF

# Cleanup
rm -f deploy.tar.gz sync-service

echo "✅ Deployment complete!"
echo ""
echo "📋 Useful commands:"
echo "  Check status:  ssh $SSH_USER@$VPS_IP 'systemctl status verifi-sync'"
echo "  View logs:     ssh $SSH_USER@$VPS_IP 'journalctl -u verifi-sync -f'"
echo "  Restart:       ssh $SSH_USER@$VPS_IP 'systemctl restart verifi-sync'"
echo "  Health check:  curl http://$VPS_IP:3001/health"
