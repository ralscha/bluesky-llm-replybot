# Systemd Installation Guide

This guide explains how to install and configure the Bluesky LLM Reply Bot as a systemd service on Debian 13.

## Prerequisites

- Debian 13 system
- Docker and Docker Compose installed
- Root or sudo access
- Built application binary in `bin/app`

## Installation Steps

### 1. Create Service User

```bash
sudo useradd -r -s /bin/false -m -d /opt/bluesky-replybot bluesky
```

### 2. Install Application Files

```bash
# Create application directory
sudo mkdir -p /opt/bluesky-replybot/bin

# Copy application binary
sudo cp bin/app /opt/bluesky-replybot/bin/
sudo chmod +x /opt/bluesky-replybot/bin/app

# Copy docker-compose.yml
sudo cp docker-compose.yml /opt/bluesky-replybot/

# Copy environment file
sudo cp .env /opt/bluesky-replybot/.env
sudo chmod 600 /opt/bluesky-replybot/.env

# Set ownership
sudo chown -R bluesky:bluesky /opt/bluesky-replybot
```

### 3. Setup Docker Compose Container

Ensure your PostgreSQL container is configured with `restart: unless-stopped` in docker-compose.yml:

```yaml
services:
  postgres:
    image: postgres:18
    restart: unless-stopped
    # ... rest of your config
```

Start the Docker container:

```bash
cd /opt/bluesky-replybot
sudo docker compose up -d
```

The container will now automatically start with Docker.

### 4. Install Systemd Service File

```bash
# Copy service file to systemd directory
sudo cp systemd/bluesky-replybot.service /etc/systemd/system/

# Set correct permissions
sudo chmod 644 /etc/systemd/system/bluesky-replybot.service

# Reload systemd daemon
sudo systemctl daemon-reload
```

### 5. Enable and Start Service

```bash
# Enable service to start on boot
sudo systemctl enable bluesky-replybot.service

# Start the bot (it will wait for PostgreSQL to be ready)
sudo systemctl start bluesky-replybot.service
```

## Service Management

### Check Service Status

```bash
# Check bot service status
sudo systemctl status bluesky-replybot.service

# Check Docker container status
docker ps | grep postgres
```

### View Logs

```bash
# View bot logs
sudo journalctl -u bluesky-replybot.service -f

# View recent logs with timestamps
sudo journalctl -u bluesky-replybot.service -n 100 --no-pager

# View PostgreSQL container logs
docker logs -f $(docker ps --filter "name=postgres" --format "{{.Names}}" | head -n1)
```
