# Namira Core

[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/docker-20.10+-blue.svg)](https://www.docker.com)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.md)

A high-performance, self-hosted quality assurance toolkit for VPN proxy configurations. Namira Core validates, benchmarks, and ranks VMess, VLESS, Trojan, and Shadowsocks connections with real TCP handshakes and latency measurements.

## 🚀 Features

- **Multi-Protocol Support**: Validates VMess, VLESS, Shadowsocks, and Trojan VPN configurations
- **Real Connectivity Testing**: Performs actual TCP handshakes, not just ping tests
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
- [Contributing](#-contributing)
- [License](#-license)
- [Acknowledgments](#-acknowledgments)

## ⚡ Quick Start

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/NaMiraNet/namira-core.git
cd namira-core

# Create .env file with your configuration
cp .env.example .env
# Edit .env with your settings

# Start the services
docker-compose up -d
```

Access the API at `http://localhost:8080`

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

- **Go 1.21+**
- **Redis 7.2+**
- **GitHub SSH key** (for GitHub integration)
- **Docker and Docker Compose** (for containerized deployment)

## ⚙️ Configuration

The application is configured via environment variables:

### Core Application Settings
| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_PORT | 8080 | Port for the HTTP server |
| SERVER_HOST | 0.0.0.0 | Host for the HTTP server |
| APP_TIMEOUT | 10s | Connection timeout per proxy test |
| MAX_CONCURRENT | 50 | Maximum concurrent connections |
| LOG_LEVEL | info | Logging level (debug, info, warn, error) |
| ENCRYPTION_KEY | - | Key for encrypting sensitive data |

### Redis Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| REDIS_ADDR | redis:6379 | Redis server address |
| REDIS_PASSWORD | - | Redis password |
| REDIS_DB | 0 | Redis database number |

### GitHub Integration
| Variable | Default | Description |
|----------|---------|-------------|
| GITHUB_SSH_KEY_PATH | /app/keys/github_deploy_key | Path to GitHub SSH key |
| GITHUB_OWNER | - | GitHub repository owner |
| GITHUB_REPO | - | GitHub repository name |

## 📦 Installation

### Using Docker Compose (Recommended)

1. Clone the repository:
   ```bash
   git clone https://github.com/NaMiraNet/namira-core.git
   cd namira-core
   ```

2. Create a `.env` file with your configuration:
   ```
   GITHUB_OWNER=your-github-username
   GITHUB_REPO=your-repo-name
   ENCRYPTION_KEY=your-encryption-key
   SSH_KEY_PATH=./path/to/your/ssh/key
   ```

3. Start the services:
   ```bash
   docker-compose up -d
   ```

### Manual Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/NaMiraNet/namira-core.git
   cd namira-core
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Build the application:
   ```bash
   make build
   ```

4. Run the application:
   ```bash
   ./bin/namira-core api
   ```

## 📚 API Documentation

### Health Check
```http
GET /api/health
```
Returns service health status.

### Check VPN Configurations Synchronously
```http
POST /api/check
Content-Type: application/json

{
  "configs": ["vmess://...", "vless://..."]
}
```

### Start Asynchronous Job
```http
POST /api/scan
Content-Type: application/json

{
  "configs": ["vmess://...", "vless://..."]
}
```

### Get Job Status
```http
GET /api/jobs/{job_id}
```

## 🔍 Example Usage

### Check VPN Configurations Synchronously

```bash
curl -X POST http://localhost:8080/api/check \
  -H "Content-Type: application/json" \
  -d '{"configs": ["vmess://..."]}'
```

### Start Asynchronous Check Job

```bash
curl -X POST http://localhost:8080/api/scan \
  -H "Content-Type: application/json" \
  -d '{"configs": ["vmess://...", "vless://..."]}'
```

### Check Job Status

```bash
curl -X GET http://localhost:8080/api/jobs/{job_id}
```

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
```


---

## Repository Essentials

### License
Distributed under the **MIT License**. See [`LICENSE`](LICENSE) for full text.

### Contributing Guidelines
A concise guide is in [`CONTRIBUTING.md`](CONTRIBUTING.md) covering style, commit message conventions (`feat:`, `fix:`, etc.), and test requirements.

### Code of Conduct
We follow the standard [Contributor Covenant v2.1](CODE_OF_CONDUCT.md). Harassment, hate speech, and spam will result in a ban.

### Issue & Pull-Request Templates
Structured templates live in `.github/ISSUE_TEMPLATE/` and `.github/PULL_REQUEST_TEMPLATE.md` and `.github/bug_report.md` ; they enforce reproducibility and checklist compliance.

### Support & Contact
* **Telegram**: <https://t.me/NamiraNet>  
* **Website**: `https://namira-web.vercel.app` .  
* **Email**: `namiranet [at] proton.me` .
* **GitHub Discussions**: open to all “how-do-I” questions.


---

## 🙏 Acknowledgments

- Go community for excellent libraries and tools
- V2Ray project for providing the foundation for VPN protocol implementation 