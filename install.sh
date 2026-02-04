#!/bin/bash

set -e
# Source .env file if it exists
if [ -f ".env" ]; then
    source .env
fi

# Record start time
SCRIPT_START_TIME=$(date +%s)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
BINARY_NAME="ai-gateway"
INSTALL_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"
CONFIG_DIR="/etc/ai-gateway"
SERVICE_USER="ai-gateway"

# SSH Configuration (can be overridden via environment variables)
SSH_HOST="${SSH_HOST:-}"
SSH_USER="${SSH_USER:-root}"
SSH_KEY="${SSH_KEY:-}"
SSH_PORT="${SSH_PORT:-22}"
REMOTE_TMP_DIR="/tmp/ai-gateway-install"

# Functions
print_info() {
    local current_time=$(date +%s)
    local elapsed=$((current_time - SCRIPT_START_TIME))
    local timestamp=$(date '+%H:%M:%S')
    echo -e "${GREEN}[INFO ${timestamp} +${elapsed}s]${NC} $1"
}

print_error() {
    local current_time=$(date +%s)
    local elapsed=$((current_time - SCRIPT_START_TIME))
    local timestamp=$(date '+%H:%M:%S')
    echo -e "${RED}[ERROR ${timestamp} +${elapsed}s]${NC} $1"
}

print_warning() {
    local current_time=$(date +%s)
    local elapsed=$((current_time - SCRIPT_START_TIME))
    local timestamp=$(date '+%H:%M:%S')
    echo -e "${YELLOW}[WARNING ${timestamp} +${elapsed}s]${NC} $1"
}

# Build the binary (local architecture)
build() {
    print_info "Building $BINARY_NAME..."
    go build -o $BINARY_NAME .
    print_info "Build complete!"
}

# Build the binary for Linux x86_64 (for remote deployment)
build_linux() {
    print_info "Building $BINARY_NAME for Linux x86_64..."
    GOOS=linux GOARCH=amd64 go build -o $BINARY_NAME .
    print_info "Build complete!"
}

# Install the binary
install() {
    if [ ! -f "$BINARY_NAME" ]; then
        print_error "Binary not found. Run './install.sh build' first."
        exit 1
    fi

    print_info "Installing $BINARY_NAME to $INSTALL_DIR..."
    sudo cp $BINARY_NAME $INSTALL_DIR/
    sudo chmod +x $INSTALL_DIR/$BINARY_NAME
    print_info "Installation complete!"
}

# Execute command on remote server via SSH
ssh_exec() {
    local cmd="$1"
    local ssh_opts=""

    if [ -n "$SSH_KEY" ]; then
        ssh_opts="-i $SSH_KEY"
    fi

    if [ -n "$SSH_PORT" ]; then
        ssh_opts="$ssh_opts -p $SSH_PORT"
    fi

    ssh $ssh_opts ${SSH_USER}@${SSH_HOST} "$cmd"
}

# Copy file to remote server via SCP
scp_copy() {
    local src="$1"
    local dst="$2"
    local ssh_opts=""

    if [ -n "$SSH_KEY" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY"
    fi

    if [ -n "$SSH_PORT" ]; then
        ssh_opts="$ssh_opts -P $SSH_PORT"
    fi

    # Add compression and faster cipher for better performance
    ssh_opts="$ssh_opts -C -c aes128-gcm@openssh.com"
    ssh_opts="$ssh_opts -l 65536"  # Increase SSH channel limit
    ssh_opts="$ssh_opts -o IPQoS=throughput"  # Optimize for throughput
    # ssh_opts="$ssh_opts -o TcpRcvBuf=1048576"  # 1MB TCP receive buffer
    # ssh_opts="$ssh_opts -o TcpSndBuf=1048576"  # 1MB TCP send buffer

    scp $ssh_opts "$src" ${SSH_USER}@${SSH_HOST}:"$dst"
}

# Copy all deployment files to remote server via tar pipeline
tar_copy() {
    local ssh_opts=""

    if [ -n "$SSH_KEY" ]; then
        ssh_opts="$ssh_opts -i $SSH_KEY"
    fi

    if [ -n "$SSH_PORT" ]; then
        ssh_opts="$ssh_opts -p $SSH_PORT"
    fi

    # Use optimized SSH options for large file transfers
    ssh_opts="$ssh_opts -o ServerAliveInterval=15 -o ServerAliveCountMax=3"

    # Create tar archive with all needed files and stream to remote
    print_info "Creating deployment archive and streaming to remote server..."
    tar czf - --format=ustar -C . "$BINARY_NAME" $( [ -f "ai-gateway.service" ] && echo "ai-gateway.service" ) $( [ -f "config.yaml" ] && echo "config.yaml" ) 2>/dev/null | \
    ssh $ssh_opts ${SSH_USER}@${SSH_HOST} "tar xzf - -C $REMOTE_TMP_DIR"
}

# Remove old service version on remote
remove_old_service() {
    print_info "Removing old service version..."

    # Stop and disable service if it exists
    ssh_exec "systemctl stop $BINARY_NAME 2>/dev/null || true"
    ssh_exec "systemctl disable $BINARY_NAME 2>/dev/null || true"

    # Remove old binary
    ssh_exec "rm -f $INSTALL_DIR/$BINARY_NAME"

    # Remove old service file
    ssh_exec "rm -f $SERVICE_DIR/$BINARY_NAME.service"

    # Reload systemd
    ssh_exec "systemctl daemon-reload"

    print_info "Old service removed"
}

# Deploy to remote server
deploy() {
    if [ -z "$SSH_HOST" ]; then
        print_error "SSH_HOST is not set. Set it via environment variable: SSH_HOST=example.com"
        exit 1
    fi

    print_info "Deploying to remote server: ${SSH_USER}@${SSH_HOST}"

    # Build for Linux x86_64 (Ubuntu)
    build_linux

    if [ ! -f "$BINARY_NAME" ]; then
        print_error "Binary not found after build"
        exit 1
    fi

    # Verify it's a Linux binary
    file_type=$(file "$BINARY_NAME" 2>/dev/null || echo "")
    if [[ "$file_type" == *"Linux"* ]] || [[ "$file_type" == *"ELF"* ]]; then
        print_info "Binary built for Linux: $file_type"
    else
        print_warning "Binary file type: $file_type (expected Linux ELF)"
    fi

    # Create temporary directory on remote
    print_info "Preparing remote server..."
    ssh_exec "mkdir -p $REMOTE_TMP_DIR"

    # Copy all files to remote using tar pipeline
    tar_copy

    # Remove old service
    remove_old_service

    # Install on remote
    print_info "Installing on remote server..."
    ssh_exec "bash -s" <<'REMOTE_INSTALL'
        set -e

        BINARY_NAME="ai-gateway"
        INSTALL_DIR="/usr/local/bin"
        SERVICE_DIR="/etc/systemd/system"
        CONFIG_DIR="/etc/ai-gateway"
        SERVICE_USER="ai-gateway"
        REMOTE_TMP_DIR="/tmp/ai-gateway-install"

        # Create service user if it doesn't exist
        if ! id "$SERVICE_USER" &>/dev/null; then
            useradd -r -s /bin/false $SERVICE_USER
        fi

        # Create config directory
        mkdir -p $CONFIG_DIR
        chown $SERVICE_USER:$SERVICE_USER $CONFIG_DIR
        chmod 755 $CONFIG_DIR

        # Install binary
        cp $REMOTE_TMP_DIR/$BINARY_NAME $INSTALL_DIR/
        chmod +x $INSTALL_DIR/$BINARY_NAME
        chown root:root $INSTALL_DIR/$BINARY_NAME

        # Install service file
        if [ -f "$REMOTE_TMP_DIR/ai-gateway.service" ]; then
            cp $REMOTE_TMP_DIR/ai-gateway.service $SERVICE_DIR/
        else
            # Create service file inline
            cat > $SERVICE_DIR/ai-gateway.service <<EOF
[Unit]
Description=AI Gateway - OpenAI-compatible API gateway
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$INSTALL_DIR/$BINARY_NAME
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$CONFIG_DIR

# Environment
Environment="PATH=/usr/local/bin:/usr/bin:/bin"

[Install]
WantedBy=multi-user.target
EOF
        fi

        # Install config file if provided
        if [ -f "$REMOTE_TMP_DIR/config.yaml" ]; then
            cp $REMOTE_TMP_DIR/config.yaml $CONFIG_DIR/
            chown $SERVICE_USER:$SERVICE_USER $CONFIG_DIR/config.yaml
            chmod 600 $CONFIG_DIR/config.yaml
        fi

        # Reload systemd
        systemctl daemon-reload

        # Enable and start service
        systemctl enable $BINARY_NAME
        systemctl start $BINARY_NAME

        # Cleanup temp directory
        rm -rf $REMOTE_TMP_DIR

        echo "Installation complete!"
REMOTE_INSTALL

    # Check service status
    print_info "Checking service status..."
    ssh_exec "systemctl status $BINARY_NAME --no-pager -l" || true

    print_info "Deployment complete!"
    print_info "Service is running on ${SSH_HOST}"
}

# Install systemd service (local)
install_service() {
    if [ ! -f "$BINARY_NAME" ]; then
        print_error "Binary not found. Run './install.sh build' first."
        exit 1
    fi

    # Create service user if it doesn't exist
    if ! id "$SERVICE_USER" &>/dev/null; then
        print_info "Creating service user: $SERVICE_USER"
        sudo useradd -r -s /bin/false $SERVICE_USER
    fi

    # Create config directory
    print_info "Creating config directory: $CONFIG_DIR"
    sudo mkdir -p $CONFIG_DIR
    sudo chown $SERVICE_USER:$SERVICE_USER $CONFIG_DIR
    sudo chmod 755 $CONFIG_DIR

    # Copy config file if it doesn't exist
    if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
        if [ -f "config.yaml" ]; then
            print_info "Copying config file to $CONFIG_DIR"
            sudo cp config.yaml $CONFIG_DIR/
            sudo chown $SERVICE_USER:$SERVICE_USER $CONFIG_DIR/config.yaml
            sudo chmod 600 $CONFIG_DIR/config.yaml
        else
            print_warning "No config.yaml found. Please create one at $CONFIG_DIR/config.yaml"
        fi
    fi

    # Install binary
    install

    # Create systemd service file
    print_info "Creating systemd service file..."
    sudo tee $SERVICE_DIR/ai-gateway.service > /dev/null <<EOF
[Unit]
Description=AI Gateway - OpenAI-compatible API gateway
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
ExecStart=$INSTALL_DIR/$BINARY_NAME
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$CONFIG_DIR

# Environment
Environment="PATH=/usr/local/bin:/usr/bin:/bin"

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    print_info "Reloading systemd daemon..."
    sudo systemctl daemon-reload

    print_info "Service installed! Use the following commands to manage it:"
    echo "  sudo systemctl start ai-gateway"
    echo "  sudo systemctl enable ai-gateway"
    echo "  sudo systemctl status ai-gateway"
}

# Run tests
test() {
    print_info "Running tests..."
    go test -v ./...
    print_info "Tests complete!"
}

# Run tests with coverage
test_coverage() {
    print_info "Running tests with coverage..."
    go test -cover ./...
    print_info "Coverage report complete!"
}

# Show usage
usage() {
    echo "Usage: $0 {build|install|install-service|deploy|test|test-coverage}"
    echo ""
    echo "Commands:"
    echo "  build           - Build the binary"
    echo "  install         - Install the binary to $INSTALL_DIR (local)"
    echo "  install-service - Install binary and systemd service (local)"
    echo "  deploy          - Build and deploy to remote server via SSH"
    echo "  test            - Run tests"
    echo "  test-coverage   - Run tests with coverage report"
    echo ""
    echo "Remote Deployment (for 'deploy' command):"
    echo "  SSH_HOST        - Remote server hostname or IP (required)"
    echo "  SSH_USER        - SSH user (default: root)"
    echo "  SSH_KEY         - Path to SSH private key (optional)"
    echo "  SSH_PORT        - SSH port (default: 22)"
    echo ""
    echo "Example:"
    echo "  SSH_HOST=example.com SSH_USER=deploy ./install.sh deploy"
    exit 1
}

# Main
case "$1" in
    build)
        build
        ;;
    install)
        install
        ;;
    install-service)
        install_service
        ;;
    deploy)
        deploy
        ;;
    test)
        test
        ;;
    test-coverage)
        test_coverage
        ;;
    *)
        usage
        ;;
esac