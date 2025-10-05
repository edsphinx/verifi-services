#!/bin/bash

set -e

# Configuration
VPS_IP="198.144.183.32"
VPS_USER="root"
SERVICE_NAME="verifi-indexer"
DEPLOY_DIR="/opt/verifi-indexer"

echo "🚀 Deploying VeriFi Indexer Service to VPS..."

# Build the binary locally
echo "📦 Building Go binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o indexer ./cmd/server

# Create deployment package
echo "📦 Creating deployment package..."
tar czf deploy.tar.gz indexer

# Copy to VPS
echo "📤 Uploading to VPS..."
scp deploy.tar.gz ${VPS_USER}@${VPS_IP}:/tmp/

# Deploy on VPS
echo "🔧 Installing on VPS..."
ssh ${VPS_USER}@${VPS_IP} << 'ENDSSH'
set -e

# Create directory
sudo mkdir -p /opt/verifi-indexer
cd /opt/verifi-indexer

# Extract binary
sudo tar xzf /tmp/deploy.tar.gz
sudo chmod +x indexer

# Copy environment variables from main project
if [ -f /opt/verifi-protocol/.env ]; then
    sudo cp /opt/verifi-protocol/.env .env
fi

# Create systemd service
sudo tee /etc/systemd/system/verifi-indexer.service > /dev/null << 'EOF'
[Unit]
Description=VeriFi Event Indexer Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/verifi-indexer
Environment="PATH=/usr/local/bin:/usr/bin:/bin"
EnvironmentFile=/opt/verifi-indexer/.env
ExecStart=/opt/verifi-indexer/indexer
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
sudo systemctl daemon-reload

# Enable and start service
sudo systemctl enable verifi-indexer
sudo systemctl restart verifi-indexer

# Show status
sudo systemctl status verifi-indexer --no-pager

# Cleanup
rm /tmp/deploy.tar.gz

echo "✅ Deployment complete!"
ENDSSH

# Cleanup local files
rm deploy.tar.gz indexer

echo ""
echo "✅ VeriFi Indexer Service deployed successfully!"
echo ""
echo "📊 Service Management Commands:"
echo "  sudo systemctl status verifi-indexer   # Check status"
echo "  sudo systemctl restart verifi-indexer  # Restart service"
echo "  sudo systemctl stop verifi-indexer     # Stop service"
echo "  sudo journalctl -u verifi-indexer -f   # View logs"
echo ""
echo "🌐 Service URL: http://${VPS_IP}:3002"
echo "🏥 Health Check: http://${VPS_IP}:3002/health"
echo "📊 Status: http://${VPS_IP}:3002/status"
