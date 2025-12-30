# Telegram Auto Check-in

[English](#english) | [中文](README-CN.md)

A Go-based automated Telegram check-in program supporting multiple accounts, flexible task scheduling, and various check-in methods.

## Features

- **Multi-Account Support** - Manage multiple Telegram accounts simultaneously
- **Flexible Login Methods** - Phone number or QR code authentication with 2FA support
- **Multiple Check-in Methods** - Text messages or button clicks
- **Concurrent Execution** - High-performance worker pool architecture
- **Flexible Scheduling** - Cron expressions and interval-based task scheduling
- **Proxy Support** - SOCKS5 proxy configuration
- **Session Persistence** - Automatic session management after first login
- **Comprehensive Logging** - Main log and separate task logs
- **Docker Ready** - Official multi-arch Docker images available
- **Internationalization** - English and Chinese language support

## Quick Start

### Option 1: Docker (Recommended)

```bash
# Pull the latest image
docker pull ghcr.io/bamzest/telegram-auto-checkin:latest

# Create and edit configuration file
cp config.yaml config.local.yaml
# Edit config.local.yaml with your settings

# Run the container
docker run --rm -it \
  -v $(pwd)/session:/app/session \
  -v $(pwd)/config.local.yaml:/app/config.yaml:ro \
  -v $(pwd)/log:/app/log \
  ghcr.io/bamzest/telegram-auto-checkin:latest

# Or use docker-compose
docker-compose up -d
```

### Option 2: Pre-built Binary

Download the latest release from [GitHub Releases](https://github.com/bamzest/telegram-auto-checkin/releases).

```bash
# Extract the archive
tar -xzf telegram-auto-checkin_*.tar.gz

# Configure the application
cp config.yaml config.local.yaml
# Edit config.local.yaml with your settings

# Run the binary
./telegram-auto-checkin --config config.local.yaml
```

### Option 3: Build from Source

```bash
# Requirements: Go 1.20 or higher
git clone https://github.com/bamzest/telegram-auto-checkin.git
cd telegram-auto-checkin

# Build the binary
go build -o telegram-auto-checkin .

# Run the application
./telegram-auto-checkin
```

## Configuration

All configuration options are documented in [config.yaml](config.yaml). Key sections include:

### Basic Settings

- **Language**: Set `language: "en"` for English or `"zh"` for Chinese
- **Proxy**: Optional SOCKS5 proxy address (e.g., `127.0.0.1:1080`)
- **App Credentials**: Obtain from https://my.telegram.org/apps
  - `app_id`: Your Telegram API ID
  - `app_hash`: Your Telegram API hash

### Account Configuration

Configure multiple accounts with individual settings:

```yaml
accounts:
  - name: "main"
    phone: "+1234567890"        # For phone number login
    password: "your_2fa_pass"   # Optional: 2FA password
    tasks:
      - name: "daily_checkin"
        enabled: true
        target: "@botusername"
        method: "message"       # or "button"
        payload: "/checkin"
        schedule: "0 8 * * *"   # Cron expression
```

### Login Methods

**Phone Number Login**:
- Provide `phone` in configuration
- Enter verification code when prompted
- Optionally set `password` for 2FA accounts

**QR Code Login**:
- Leave `phone` empty
- Scan the `tg://login?token=...` link displayed in terminal
- Confirm login on your mobile device

### Task Scheduling

Tasks support flexible scheduling options:

- **Cron expressions**: `"0 8 * * *"` (8 AM daily)
- **Interval syntax**: `"@every 12h"` (every 12 hours)
- **Run on start**: Set `run_on_start: true` for immediate execution

## Configuration Priority

1. Environment variables (highest priority)
2. Environment-specific config file: `config.{APP_ENV}.yaml`
3. Main configuration file: `config.yaml`

## Logging

- **Main log**: `log/app.log`
- **Task logs**: `log/tasks/{account}/{task}_{timestamp}.log`
- Configurable log directory and format in `config.yaml`

## Development

### Project Structure

```
telegram-auto-checker/
├── main.go                 # Application entry point
├── config.yaml             # Configuration template
├── internal/               # Internal packages
│   ├── client/            # Telegram client wrapper
│   ├── config/            # Configuration management
│   ├── executor/          # Task execution engine
│   ├── scheduler/         # Task scheduling
│   ├── logger/            # Logging utilities
│   └── i18n/              # Internationalization
├── locales/               # Translation files
│   ├── en.yaml           # English translations
│   └── zh.yaml           # Chinese translations
└── session/              # Session storage (generated)
```

### Building

```bash
# Install dependencies
go mod tidy

# Build for current platform
go build -o telegram-auto-checkin .

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o telegram-auto-checkin .
```

## Docker Usage

### Using Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: '3'
services:
  telegram-auto-checkin:
    image: ghcr.io/bamzest/telegram-auto-checkin:latest
    container_name: telegram-auto-checkin
    volumes:
      - ./session:/app/session
      - ./config.local.yaml:/app/config.yaml:ro
      - ./log:/app/log
    restart: unless-stopped
```

Run with:
```bash
docker-compose up -d
```

### Multi-Architecture Support

Docker images are available for:
- `linux/amd64` (x86_64)
- `linux/arm64` (ARM 64-bit)
- `linux/arm/v7` (ARM 32-bit)

## License

MIT License - see [LICENSE](LICENSE) for details.

## Author

**bamzest**

## Disclaimer

This tool is for educational and automation purposes only. Users are responsible for compliance with Telegram's Terms of Service. The author assumes no liability for any misuse or violations.

