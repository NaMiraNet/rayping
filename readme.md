# **Namira Core**  
*A fast, self-hosted quality-assurance toolkit for VMess / VLESS / Trojan / Shadowsocks links*

---

<!-- TOC generated with https://github.com/ekalinin/github-markdown-toc -->
- [Overview & Purpose](#overview--purpose)
- [For Non-Technical Users (Plain English Walk-Through)](#for-non-technical-users-plain-english-walk-through)
- [Installation & Build Instructions](#installation--build-instructions)
  - [Quick Docker Run (2 minutes)](#quick-docker-run-2-minutes)
  - [Permanent API Stack (≈ 15 minutes)](#permanent-api-stack--15-minutes)
  - [Building from Source (optional)](#building-from-source-optional)
  - [Common Errors & Fixes](#common-errors--fixes)
- [Usage Guide](#usage-guide)
  - [CLI One-Shot Scan](#cli-one-shot-scan)
  - [REST API Workflow](#rest-api-workflow)
  - [Advanced Options & Configuration](#advanced-options--configuration)
- [Maintenance & Community](#maintenance--community)
- [Repository Essentials](#repository-essentials)
  - [License](#license)
  - [Contributing Guidelines](#contributing-guidelines)
  - [Code of Conduct](#code-of-conduct)
  - [Issue & Pull-Request Templates](#issue--pull-request-templates)
  - [Support & Contact](#support--contact)
- [Release Cycle & Versioning](#release-cycle--versioning)

---

## Overview & Purpose
**Namira Core** is a lightweight service that **verifies, benchmarks, and ranks VPN subscription links**.  
Instead of trusting a random list of `vmess://` or `ss://` URLs, Namira Core:

1. **Parses** each link to ensure it is syntactically valid.  
2. **Establishes** a minimal real TCP handshake (far more reliable than a simple ICMP ping).  
3. **Measures** latency in milliseconds to reveal practical performance.  
4. **Outputs** a clean JSON file or pushes the results to GitHub / Telegram.

> **Why does this matter?**  
> Latency and availability vary wildly between nodes. Manual testing is slow, error-prone, and often skips subtle misconfigurations. Namira Core automates the process, giving quantifiable data so you can choose the fastest, truly alive servers.

---

## For Non-Technical Users (Plain English Walk-Through)

> *Analogy*: Think of Namira Core as a **speed-gun and health-check booth** for VPN passes.

1. **Drop your pass pile** (`links.txt`) into the booth.  
2. The booth **inspects each pass** to see if the barcode is even readable (format check).  
3. It then **tries the door** with each pass—no guessing—verifying which doors actually open (TCP handshake).  
4. A radar gun **records how quickly** the door opens (latency).  
5. Finally, it **hands you a tidy report**: green passes are fast & valid, red ones are duds.

### Prerequisites in everyday terms
| What you need | Why you need it | Real-world comparison |
|---------------|-----------------|-----------------------|
| **Docker** **20.10+**           | Container runtime that isolates apps | Like installing a sandbox where the tool lives |
| **A text file** with links      | The “stack of passes” to test        | Simply a list of URLs copied from your provider |
| *(API mode only)* **Redis**     | Temporary queue for jobs             | A fast notepad the service scribbles on |

---

## Installation & Build Instructions

### Quick Docker Run (2 minutes)

```bash
docker run --rm -v "$PWD:/data" ghcr.io/namiranet/namira-core:latest   check --config /data/links.txt
# Results saved to ./report.json
```

> **Tip**: Pipe the JSON through `jq` for pretty colours:  
> `cat report.json | jq '.'`

---

### Permanent API Stack (≈ 15 minutes)

```bash
git clone https://github.com/NaMiraNet/namira-core.git
cd namira-core
cp .env.example .env        # set at least API_KEY
docker compose up --build -d
curl http://localhost:8080/health
```

| Container | Port | Purpose |
|-----------|------|---------|
| `namira-core` | 8080 | REST API & worker orchestrator |
| `redis` | 6379 | Job queue backend |

---

### Building from Source (optional)

```bash
# Requires: Go 1.22+, make, git, (optional) upx
git clone https://github.com/NaMiraNet/namira-core.git
cd namira-core
make test        # run unit tests
make build       # static binary in ./bin/namira-core
```

*Binary size*: ~8 MB after UPX compression—runs on any distro with glibc.

---

### Common Errors & Fixes

| Symptom | Likely Cause | Remedy |
|---------|--------------|--------|
| Container exits instantly | Mis-set environment variable | `docker logs namira-core` then review `.env` |
| Every link `timeout` | Outbound firewall blocks TCP-80/443 | Open those ports or move behind less restrictive network |
| `401 Unauthorized` from API | Missing `X-API-Key` header | Add correct header value |
| High latency numbers | `CHECK_HOST` too far away | Pick geolocated IP, e.g. `8.8.8.8:53` |

---

## Usage Guide

### CLI One-Shot Scan

```bash
./bin/namira-core check --config links.txt --timeout 10s --concurrency 50
```

### REST API Workflow

```bash
# Submit job
curl -X POST http://localhost:8080/scan      -H "X-API-Key: $API_KEY"      -H "Content-Type: application/json"      -d '{"configs": ["trojan://...", "vmess://..."]}'

# Poll status
curl http://localhost:8080/job/<job_id>

# Fetch final JSON
curl http://localhost:8080/job/<job_id>/result > result.json
```

### Advanced Options & Configuration

| Variable / Flag | Default | Description |
|-----------------|---------|-------------|
| `--timeout, APP_TIMEOUT` | `10s` | Per-link dial timeout |
| `--concurrency, MAX_CONCURRENT` | `50` | Parallel workers |
| `CHECK_HOST` | `1.1.1.1:80` | Destination for latency probe |
| `TELEGRAM_BOT_TOKEN` & `TELEGRAM_CHANNEL` | *empty* | Enable Telegram notifications |
| `ENCRYPTION_KEY` | *nil* | AES-256 key used when pushing results to GitHub |

---

## Maintenance & Community

* **Bug reports** – Open a GitHub Issue and attach the offending link (redact sensitive parts).  
* **Feature requests** – Label the Issue as *enhancement*; discussion is welcome.  
* **Code contributions** – Fork, create a feature branch, run `make test`, open a PR.  
* **Review cadence** – Maintainers triage weekly; merges follow semantic version tags.  

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

*Built with a healthy dose of skepticism—every link must prove its worth.*
