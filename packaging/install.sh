#!/bin/bash
set -e

# Meeting Siren Installation Script
# Supports systemd (Linux) and launchd (macOS)

INSTALL_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"
LAUNCHD_DIR="/Library/LaunchDaemons"
CONFIG_DIR="/etc"
STATE_DIR="/var/lib/meeting-siren"
LOG_DIR="/var/log"
USER="meeting-siren"
GROUP="meeting-siren"

# macOS specific directories
MACOS_STATE_DIR="/usr/local/var/lib/meeting-siren"
MACOS_LOG_DIR="/usr/local/var/log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (use sudo)"
        exit 1
    fi
}

detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        OS="linux"
        log_info "Detected Linux system"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        log_info "Detected macOS system"
        # Use macOS specific directories
        STATE_DIR="$MACOS_STATE_DIR"
        LOG_DIR="$MACOS_LOG_DIR"
    else
        log_error "Unsupported operating system: $OSTYPE"
        exit 1
    fi
}

install_binary() {
    log_info "Installing meeting-siren binary..."

    # Check if binary exists in current directory
    if [[ ! -f "./meeting-siren" ]]; then
        log_error "meeting-siren binary not found in current directory"
        exit 1
    fi

    # Copy binary to install directory
    cp ./meeting-siren "$INSTALL_DIR/meeting-siren"
    chmod +x "$INSTALL_DIR/meeting-siren"

    log_info "Binary installed to $INSTALL_DIR/meeting-siren"
}

create_user() {
    if [[ "$OS" == "linux" ]]; then
        # Create system user for Linux
        if ! id "$USER" &>/dev/null; then
            log_info "Creating system user: $USER"
            useradd --system --shell /bin/false --home-dir "$STATE_DIR" --create-home "$USER"
        else
            log_info "User $USER already exists"
        fi
    else
        # macOS doesn't need a system user for launchd services
        log_info "Skipping user creation on macOS"
    fi
}

create_directories() {
    log_info "Creating directories..."

    # Create state directory
    mkdir -p "$STATE_DIR"

    # Create log directory if it doesn't exist
    mkdir -p "$LOG_DIR"

    if [[ "$OS" == "linux" ]]; then
        # Set ownership on Linux
        chown -R "$USER:$USER" "$STATE_DIR"
        chmod 755 "$STATE_DIR"
    else
        # Set permissions on macOS
        chmod 755 "$STATE_DIR"
        chmod 755 "$LOG_DIR"
    fi

    log_info "Directories created: $STATE_DIR"
}

install_service() {
    if [[ "$OS" == "linux" ]]; then
        install_systemd_service
    else
        install_launchd_service
    fi
}

install_systemd_service() {
    log_info "Installing systemd service..."

    if [[ ! -f "./packaging/meeting-siren.service" ]]; then
        log_error "systemd service file not found: ./packaging/meeting-siren.service"
        exit 1
    fi

    # Copy service file
    cp ./packaging/meeting-siren.service "$SERVICE_DIR/meeting-siren.service"

    # Reload systemd and enable service
    systemctl daemon-reload
    systemctl enable meeting-siren.service

    log_info "Systemd service installed and enabled"
    log_info "Start with: sudo systemctl start meeting-siren"
    log_info "View logs with: sudo journalctl -u meeting-siren -f"
}

install_launchd_service() {
    log_info "Installing launchd service..."

    if [[ ! -f "./packaging/com.meeting-siren.plist" ]]; then
        log_error "launchd plist file not found: ./packaging/com.meeting-siren.plist"
        exit 1
    fi

    # Copy plist file
    cp ./packaging/com.meeting-siren.plist "$LAUNCHD_DIR/com.meeting-siren.plist"

    # Set correct permissions
    chown root:wheel "$LAUNCHD_DIR/com.meeting-siren.plist"
    chmod 644 "$LAUNCHD_DIR/com.meeting-siren.plist"

    # Load the service
    launchctl load "$LAUNCHD_DIR/com.meeting-siren.plist"

    log_info "Launchd service installed and loaded"
    log_info "Start with: sudo launchctl start com.meeting-siren"
    log_info "View logs in: $LOG_DIR/meeting-siren.log"
}

install_config() {
    log_info "Installing example configuration..."

    if [[ -f "./configs/meeting-siren.example.yaml" ]]; then
        cp ./configs/meeting-siren.example.yaml "$CONFIG_DIR/meeting-siren.yaml.example"
        log_info "Example config installed to $CONFIG_DIR/meeting-siren.yaml.example"
        log_warn "Please copy and customize: cp $CONFIG_DIR/meeting-siren.yaml.example $CONFIG_DIR/meeting-siren.yaml"
    else
        log_warn "Example configuration file not found"
    fi
}

show_next_steps() {
    log_info "Installation completed successfully!"
    echo
    log_info "Next steps:"
    echo "1. Configure your NATS connection and audio settings"

    if [[ "$OS" == "linux" ]]; then
        echo "   - Edit /etc/meeting-siren.yaml or set environment variables"
        echo "2. Start the service: sudo systemctl start meeting-siren"
        echo "3. Check status: sudo systemctl status meeting-siren"
        echo "4. View logs: sudo journalctl -u meeting-siren -f"
    else
        echo "   - Edit /etc/meeting-siren.yaml or set environment variables"
        echo "2. Start the service: sudo launchctl start com.meeting-siren"
        echo "3. Check if running: sudo launchctl list | grep meeting-siren"
        echo "4. View logs: tail -f $LOG_DIR/meeting-siren.log"
    fi

    echo
    log_info "Test the installation by publishing a NATS message:"
    echo "nats pub alerts.meeting.alarm '{\"title\":\"Test Meeting\",\"when\":\"$(date -u +%Y-%m-%dT%H:%M:%S+00:00)\",\"lead\":5}'"
}

main() {
    log_info "Starting meeting-siren installation..."

    check_root
    detect_os
    install_binary
    create_user
    create_directories
    install_service
    install_config
    show_next_steps
}

# Handle command line arguments
case "${1:-}" in
    "uninstall")
        log_info "Uninstalling meeting-siren..."
        if [[ "$OS" == "linux" ]]; then
            systemctl stop meeting-siren || true
            systemctl disable meeting-siren || true
            rm -f "$SERVICE_DIR/meeting-siren.service"
            systemctl daemon-reload
        else
            launchctl unload "$LAUNCHD_DIR/com.meeting-siren.plist" || true
            rm -f "$LAUNCHD_DIR/com.meeting-siren.plist"
        fi
        rm -f "$INSTALL_DIR/meeting-siren"
        log_info "Uninstallation completed"
        ;;
    *)
        main
        ;;
esac