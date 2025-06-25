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

## 📦 Installation

### Using Docker Compose (Recommended)

1. Clone the repository:
   ```bash
   git clone https://github.com/NaMiraNet/namira-core.git
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
   git clone https://github.com/NaMiraNet/namira-core.git
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
test                 Run all tests ---------------------------- Coming Soon --------------------------------
test-coverage        Run tests with coverage ----------------------------- Coming Soon -------------------------
up                   Start Docker Compose services
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
http
POST /scan
Content-Type: text/plain

vmess://xxxxxxxxxxxxxxxxxxxx
vless://yyyyyyyyyyyyyyyyyyyy
trojan://zzzzzzzzzzzzzzzzzz


###### Curl Example :
http
curl -X POST http://localhost:8080/scan \
  -H "Content-Type: text/plain" \
  --data-binary @proxies.txt

##### proxies.txt
txt
vmess://xxxxxxxxxxxxxxxxxxxx
vless://yyyyyyyyyyyyyyyyyyyy
trojan://zzzzzzzzzzzzzzzzzz



### Get Job Status
http
GET /jobs/{job_id}


## 🔍 Example Usage


### Start Asynchronous Check Job

bash
curl -X POST http://localhost:8080/scan \
  -H "Content-Type: application/json" \
  -d '{"configs": ["vmess://...", "vless://..."]}'


### Check Job Status

bash
curl -X GET http://localhost:8080/jobs/{job_id}


## 🛠️ Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| All connections timeout | Firewall blocking outbound connections | Open required ports or test from different network |
| Redis connection failed | Redis not running or wrong connection string | Verify Redis is running and configuration is correct |
| SSH connectivity test failed | Invalid SSH key or permissions | Check SSH key path and permissions |

### Debug Mode

Enable debug logging:

bash
export LOG_LEVEL=debug
./bin/namira-core api



---

## Repository Essentials

### License
Distributed under the **MIT License**. See [LICENSE](LICENSE) for full text.

### Contributing Guidelines
A concise guide is in [CONTRIBUTING.md](CONTRIBUTING.md) covering style, commit message conventions (feat:, fix:, etc.), and test requirements.

### Code of Conduct
We follow the standard [Contributor Covenant v2.1](CODE_OF_CONDUCT.md). Harassment, hate speech, and spam will result in a ban.

### Issue & Pull-Request Templates
Structured templates live in .github/ISSUE_TEMPLATE/ and .github/PULL_REQUEST_TEMPLATE.md and .github/bug_report.md ; they enforce reproducibility and checklist compliance.
---

## 🙏 Acknowledgments

- Go community for excellent libraries and tools
- V2Ray project for providing the foundation for VPN protocol implementation 

---
### Support & Contact
* **Telegram**: <https://t.me/NamiraNet>  
* **Website**: https://namira-web.vercel.app .  
* **Email**: namiranet [at] proton.me .
* **GitHub Discussions**: open to all “how-do-I” questions.

---

<h3 align='center'> 🤝 Contributors </h3>

<div align="center">
  <a href="https://github.com/NaMiraNet/namira-core/graphs/contributors">
    <img src="https://contrib.rocks/image?repo=NaMiraNet/namira-core" alt="Contributors" />
  </a>
  <br/>
  <sub>Made with ❤️ by the community using <a href="https://contrib.rocks">contrib.rocks</a></sub>
  <br/><br/>
  <strong>🙏 Thank you to all the contributors who make this project possible.</strong>
</div>