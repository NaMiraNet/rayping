# Namira Core

[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/docker-20.10+-blue.svg)](https://www.docker.com)
[![License](https://img.shields.io/badge/license-AGPL3-green.svg)](LICENSE)

A high-performance, self-hosted quality assurance toolkit for VPN proxy configurations. Namira Core validates, benchmarks, and ranks VMess, VLESS, Trojan, and Shadowsocks connections with real TCP handshakes and latency measurements.

## 🚀 Features

- **Multi-Protocol Support**: Validates VMess, VLESS, Shadowsocks, and Trojan VPN configurations
- **Real Connection Testing**: Uses real TCP handshakes, not just pings
- **High Concurrency**: Dynamically adjusts concurrent connection limits based on system resources
- **API Server**: RESTful API for checking VPN configurations
- **Notification System**: Integrated Telegram notifications for valid configurations
- **Worker Pool**: Efficient job processing with configurable worker pools
- **Redis Integration**: Persistent storage and caching of results
- **GitHub Integration**: Automated updates to GitHub repositories with valid configurations

## 📋 Table of Contents

- [Quick Start](#-quick-start)
- [How It Works](#-how-it-works)
- [Architecture](#-architecture)
- [Requirements](#-requirements)
- [Configuration](#-configuration)
- [Installation](#-installation)
- [API Documentation](#-api-documentation)
- [Example Usage](#-example-usage)
- [Troubleshooting](#-troubleshooting)
- [Project Structure](#-project-structure)
- [Contributing](#-contributing)
- [License](#-license)
- [Acknowledgments](#-acknowledgments)

## ⚡ Quick Start

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/NamiraNet/namira-core.git
cd namira-core

# Create .env file with your configuration
cp .env.example .env
# Edit .env with your settings

# Start the services
docker-compose up -d
```
Access the API at `http://localhost:8080`

### Using CLI

```bash
# Build the application
make build-local

# Check VPN configurations from file
./bin/namira-core check --file proxies.txt

# Check individual configurations
./bin/namira-core check --config "vmess://..." --config "vless://..."
```


## 🔄 How It Works

Namira Core processes VPN configurations through a pipeline of operations to validate their connectivity and measure performance:

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│             │    │             │    │             │    │             │    │             │
│    Input    │───►│   Parser    │───►│  Checker    │───►│  Analyzer   │───►│   Output    │
│  VPN Links  │    │             │    │             │    │             │    │   Results   │
│             │    │             │    │             │    │             │    │             │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                         │                  │                  │
                         ▼                  ▼                  ▼
                   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
                   │             │    │             │    │             │
                   │  Protocol   │    │    TCP      │    │  Latency    │
                   │ Extraction  │    │ Handshake   │    │ Measurement │
                   │             │    │             │    │             │
                   └─────────────┘    └─────────────┘    └─────────────┘
```

### Workflow

1. **Input Processing**: 
   - VPN configuration links are submitted via API or CLI
   - Links are queued for processing in the worker pool

2. **Parsing**:
   - Each link is parsed to extract protocol-specific parameters
   - Supported protocols: VMess, VLESS, Shadowsocks, Trojan

3. **Checking**:
   - Real TCP handshakes are performed to verify connectivity
   - Connection timeouts and errors are handled gracefully
   - Latency is measured with multiple samples for accuracy

4. **Analysis**:
   - Results are analyzed to determine availability status
   - Configurations are ranked by performance
   - Metadata is enriched (location, provider, etc.)

5. **Output**:
   - Results are returned via API or saved to files
   - Valid configurations can be automatically:
     - Sent to Telegram channels
     - Committed to GitHub repositories
     - Stored in Redis for caching

The worker pool manages concurrency, ensuring optimal resource utilization while preventing system overload.

## 🏗 Architecture

The application is structured with clean separation of concerns:

- **Core**: Central components for parsing and checking VPN configurations
- **API**: RESTful endpoints for submitting configuration check requests
- **Worker**: Background job processing for handling configuration checks
- **Notify**: Notification system for sending results via Telegram
- **Config**: Configuration management from environment variables
- **Logger**: Structured logging using zap

## 📋 Requirements

- **Go 1.24+**
- **Redis 8.0+**
- **GitHub SSH key** (for GitHub integration)
- **Docker and Docker Compose** (for containerized deployment)


## ⚙️ Configuration

The application is configured via environment variables:

### Server Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_PORT | 8080 | Port for the HTTP server |
| SERVER_HOST | localhost | Host for the HTTP server |
| SERVER_READ_TIMEOUT | 30s | Server read timeout |
| SERVER_WRITE_TIMEOUT | 30s | Server write timeout |
| SERVER_IDLE_TIMEOUT | 60s | Server idle timeout |

### Worker Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| WORKER_COUNT | 5 | Number of workers |
| WORKER_QUEUE_SIZE | 100 | Size of worker queue |

### Redis Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| REDIS_ADDR | localhost:6379 | Redis server address |
| REDIS_PASSWORD | - | Redis password |
| REDIS_DB | 0 | Redis database number |
| REDIS_RESULT_TTL | 24h | Redis result TTL |

### GitHub Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| GITHUB_OWNER | - | GitHub repository owner |
| GITHUB_REPO | - | GitHub repository name |
| SSH_KEY_PATH | ./keys/github_deploy_key | Path to GitHub SSH key for pushing result in github repo |

### App Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| LOG_LEVEL | info | Logging level **(debug, info, warn, error)** |
| APP_TIMEOUT | 10s | Connection timeout per proxy test |
| REFRESH_INTERVAL | 1h | Background refresh interval |
| MAX_CONCURRENT | 50 | Maximum concurrent connections |
| ENCRYPTION_KEY | - | Key for encrypting sensitive data |

### Telegram Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| TELEGRAM_BOT_TOKEN | - | Telegram bot token |
| TELEGRAM_CHANNEL | - | Telegram channel username or ID for forwarding |
| TELEGRAM_TEMPLATE | - | Template for Telegram messages |
| TELEGRAM_QR_CONFIG | - | QR configuration for Telegram |
| TELEGRAM_PROXY_URL | - | Proxy URL for Telegram |
| TELEGRAM_SENDING_INTERVAL | 10s | Interval between sending messages |


## 📦 Installation

### Using Docker Compose (Recommended)

1. Clone the repository:
   ```bash
   git clone https://github.com/NamiraNet/namira-core.git
   cd namira-core
   ```

2. Create a `.env` file with your configuration:
   ```bash
   cp .env.example .env
   # Edit .env with your preferred text editor
   ```

3. Start the services:
   ```bash
   make prod    # For production environment
   # OR
   make dev     # For development environment with logs
   ```

4. Check if the service is healthy:
   ```bash
   make health
   ```

### Manual Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/NamiraNet/namira-core.git
   cd namira-core
   ```

2. Install dependencies:
   ```bash
   make install
   ```

3. Build the application:
   ```bash
   make build-local
   ```

4. Run the application:
   ```bash
   make run-local
   # OR run directly
   ./bin/namira-core api
   ```

### Available Make Commands

Run `make help` to see all available commands:

```
Namira Core Makefile
Usage: make [target]

Targets:
build                Build Docker containers without starting them
build-local          Build the Go binary locally
build-release        Build optimized release binary
clean                Clean up Docker resources and build artifacts
dev                  Start development environment with Docker Compose
docker-build         Build Docker image
docker-push          Build and push Docker image
down                 Stop Docker Compose services
health               Check service health
help                 Show this help message
install              Install project dependencies
lint                 Run linters
logs                 View Docker Compose logs
prod                 Start production environment with Docker Compose
run-local            Run the application locally
test                 Run all tests / Coming Soon 
test-coverage        Run tests with coverage / Coming Soon 
up                   Start Docker Compose services
version              Show version information
```


## 💻 CLI Usage

The CLI provides a powerful interface for batch checking VPN configurations without requiring a full server setup.

### Basic Usage

```bash
# Show version
./bin/namira-core --version

# Display help
./bin/namira-core check --help

# Check configurations from a file
./bin/namira-core check --file proxies.txt

# Check individual configurations
./bin/namira-core check --config "vmess://..." --config "vless://..."

# Mix file and individual configs
./bin/namira-core check --file proxies.txt --config "trojan://..."
```

### CLI Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-i` | - | File containing VPN configurations (one per line) |
| `--config` | `-c` | - | VPN configuration strings (can be used multiple times) |
| `--format` | `-f` | `table` | Output format: `table`, `json`, `csv` |
| `--output` | `-o` | stdout | Output file path |
| `--progress` | - | `true` | Show progress during checking |
| `--concurrent` | `-j` | `10` | Maximum concurrent checks |
| `--timeout` | `-t` | `10s` | Timeout for each check |


### CLI Examples

#### Basic File Checking
```bash
# Check all configurations in a file
./bin/namira-core check --file proxies.txt

# Check with custom concurrency and timeout
./bin/namira-core check --file proxies.txt --concurrent 20 --timeout 15s
```

#### Individual Configuration Checking
```bash
# Check single configuration
./bin/namira-core check --config "vmess://eyJ2IjoiMiIsInBzIjoi..."

# Check multiple configurations
./bin/namira-core check \
  --config "vmess://eyJ2IjoiMiIsInBzIjoi..." \
  --config "vless://..." \
  --config "trojan://..."
```

#### Advanced Usage
```bash
# Silent mode with JSON output
./bin/namira-core check \
  --file proxies.txt \
  --format json \
  --output results.json \
  --progress false

# High concurrency for large files
./bin/namira-core check \
  --file large_proxy_list.txt \
  --concurrent 50 \
  --timeout 5s \
  --format csv \
  --output report.csv
```

#### Input File Format

Your input file should contain one VPN configuration per line:

```txt
vmess://eyJ2IjoiMiIsInBzIjoidGVzdCIsImFkZCI6IjEuMS4xLjEiLCJwb3J0IjoiNDQzIiwiaWQiOiJ0ZXN0LWlkIiwiYWlkIjoiMCIsInNjeSI6ImF1dG8iLCJuZXQiOiJ3cyIsInR5cGUiOiJub25lIiwiaG9zdCI6IiIsInBhdGgiOiIvIiwidGxzIjoidGxzIiwic25pIjoiIn0=
vless://test-id@2.2.2.2:443?encryption=none&security=tls&type=ws&path=/&host=#test
trojan://password@3.3.3.3:443?security=tls&type=tcp&headerType=none#test
ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@4.4.4.4:8388#test
```


## 📚 API Documentation

### Health Check
```http
GET /health
```

Returns service health status.

### 🚀 Start Asynchronous Job

This endpoint accepts proxy configurations to initiate a background scanning job.

You can submit configurations in **two formats**:

---

#### 🟢 JSON Format

```http
POST /scan
Content-Type: application/json

{
  "configs": [
    "vmess://xxxxxxxxxxxxxxxxxxxx",
    "vless://yyyyyyyyyyyyyyyyyyyy"
  ]
}
```

#### 🟢 Seperated File Format
```http
POST /scan
Content-Type: text/plain

vmess://xxxxxxxxxxxxxxxxxxxx
vless://yyyyyyyyyyyyyyyyyyyy
trojan://zzzzzzzzzzzzzzzzzz
```

###### Curl Example:
```bash
curl -X POST http://localhost:8080/scan \
  -H "Content-Type: text/plain" \
  --data-binary @proxies.txt
```

##### proxies.txt
```txt
vmess://xxxxxxxxxxxxxxxxxxxx
vless://yyyyyyyyyyyyyyyyyyyy
trojan://zzzzzzzzzzzzzzzzzz
```


### Get Job Status
```http
GET /jobs/{job_id}
```

## 🔍 Example Usage

### CLI Mode

#### Quick Check
```bash
# Check a few configurations quickly
./bin/namira-core check \
  --config "vmess://..." \
  --config "vless://..." \
  --format table
```

#### Batch Processing
```bash
# Process large list with optimal settings
./bin/namira-core check \
  --file large_list.txt \
  --concurrent 30 \
  --timeout 8s \
  --format json \
  --output results.json \
  --progress
```

### API Mode

#### Start Asynchronous Scanning

```bash
curl -X POST http://localhost:8080/scan \
  -H "Content-Type: application/json" \
  -H "X-API-KEY: apikey" \
  -d '{"configs": ["vmess://...", "vless://..."]}'
```

#### Check Job Status

```bash
curl -X GET -H "X-API-KEY: apikey" http://localhost:8080/jobs/{job_id}
```

## 🧾 Swagger Documentation

Namira Core provides OpenAPI-compliant documentation to help you explore and interact with the API.

### 📁 Swagger Files Location

* `docs/swagger.yaml` – OpenAPI specification in YAML format
* `docs/swagger.json` – OpenAPI specification in JSON format
* `docs/docs.go` – Go-based embedded Swagger handler for serving docs via the API

## 🛠️ Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| All connections timeout | Firewall blocking outbound connections | Open required ports or test from different network |
| Redis connection failed | Redis not running or wrong connection string | Verify Redis is running and configuration is correct |
| SSH connectivity test failed | Invalid SSH key or permissions | Check SSH key path and permissions |

### Debug Mode

Enable debug logging:

```bash
export LOG_LEVEL=debug
./bin/namira-core api
# OR
./bin/namira-core check --file proxies.txt
```

### CLI Specific Issues

| Issue | Solution |
|-------|----------|
| "Permission denied" when running binary | Run `chmod +x ./bin/namira-core` |
| CLI hangs on large files | Reduce `--concurrent` value or increase `--timeout` |
| Invalid output format | Use one of: `table`, `json`, `csv` |

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guidelines](CONTRIBUTING.md) before submitting a pull request.

### Development Setup

```bash
# Clone the repository
git clone https://github.com/NamiraNet/namira-core.git
cd namira-core

# Install dependencies
make install

# Run tests
make test

# Start development server
make dev
```

## 📄 License

Distributed under the **AGPL-3 License**. See [LICENSE](LICENSE) for full text.

## 🙏 Acknowledgments

- Go community for excellent libraries and tools
- V2Ray project for providing the foundation for VPN protocol implementation 

---
### Support & Contact
* **Telegram**: <https://t.me/NamiraNet>  
* **Website**: https://namira-web.vercel.app  
* **Email**: NamiraNet [at] proton.me
* **GitHub Discussions**: open to all "how-do-I" questions.

## Stargazers over time
[![Stargazers over time](https://starchart.cc/NaMiraNet/namira-core.svg?variant=light)](https://starchart.cc/NaMiraNet/namira-core)

---

<h3 align='center'> 🤝 Contributors </h3>

<div align="center">
  <a href="https://github.com/NamiraNet/namira-core/graphs/contributors">
    <img src="https://contrib.rocks/image?repo=NamiraNet/namira-core" alt="Contributors" />
  </a>
  <br/>
  <sub>Made with ❤️ by the community using <a href="https://contrib.rocks">contrib.rocks</a></sub>
  <br/><br/>
  <strong>🙏 Thank you to all the contributors who make this project possible.</strong>
</div>
