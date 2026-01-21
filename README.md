# ant2oa

> High-performance proxy service that converts OpenAI-compatible APIs to Anthropic API format

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-GPL3-blue.svg?style=flat)](LICENSE)

🌐 **Language / 语言**: [English](README.md) | [中文](README_ZH.md)

## 🎯 Overview

`ant2oa` is a high-performance Go proxy service that primarily converts **OpenAI-compatible APIs** to **Anthropic (Claude) API** format.

### Core Value

This tool serves as a "protocol converter": enabling clients and applications that originally only support Anthropic API to seamlessly call various OpenAI-compatible large language model services.

#### Supported OpenAI-Compatible Services
- 🧠 **DeepSeek** (Latest models like V3/R1)
- 🏢 **OpenAI** (GPT-4, GPT-3.5, etc.)
- 🚀 **vLLM** (Self-hosted large model services)
- 🦙 **Ollama** (Local large model inference)
- 📡 **Other OpenAI-Compatible API Services**

#### Compatible Claude Clients
- 💻 **Cursor** (AI Code Editor)
- 🤖 **Cline** (AI Programming Assistant)
- 🛠️ **Various Agent Tools**
- 📝 **Native Claude Clients**

## 🚀 Quick Start

### Method 1: Direct Run (Recommended for Testing)

1. **Download and Run**

```bash
# Clone the repository
git clone https://github.com/Pl6ME/ant2oa.git
cd ant2oa

# Run the service
go run .
```

2. **Configure Environment Variables**

Create `.env` file:

```bash
# Required Configuration
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat

# Optional Configuration
LISTEN_ADDR=:8080
RATE_LIMIT=100
```

3. **Test the Service**

```bash
curl http://localhost:8080/health
```

### Method 2: Production Deployment

#### 1. Build Executable

```bash
# Build
go build -o ant2oa .

# Or use cross-compilation for different platforms
GOOS=linux GOARCH=amd64 go build -o ant2oa-linux-amd64 .
GOOS=darwin GOARCH=amd64 go build -o ant2oa-darwin-amd64 .
GOOS=windows GOARCH=amd64 go build -o ant2oa-windows-amd64.exe .

# ARM64 architecture support (Raspberry Pi, ARM servers, etc.)
GOOS=linux GOARCH=arm64 go build -o ant2oa-linux-arm64 .
```

#### 2. Configure Service

Create `env` or `.env` configuration file:

```bash
# OpenAI-Compatible Service Configuration
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat

# Server Configuration
LISTEN_ADDR=:8080
PORT=8080

# Performance Configuration
RATE_LIMIT=200  # Requests per minute limit, 0 means unlimited

# Log Level
LOG_LEVEL=info
```

#### 3. Linux System Service Installation (Recommended for Production)

```bash
# Run with administrator privileges
sudo ./ant2oa -install

# Service Management Commands
sudo systemctl start ant2oa    # Start service
sudo systemctl stop ant2oa     # Stop service
sudo systemctl restart ant2oa  # Restart service
sudo systemctl status ant2oa   # Check service status

# View logs
journalctl -u ant2oa -f        # Real-time log viewing
journalctl -u ant2oa --since "2024-01-01" # View historical logs
```

#### 4. Docker Deployment

Create `Dockerfile`:

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o ant2oa .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/ant2oa .
COPY env .
EXPOSE 8080
CMD ["./ant2oa"]
```

Build and run:

```bash
# Build image
docker build -t ant2oa .

# Run container
docker run -d \
  --name ant2oa \
  -p 8080:8080 \
  --env-file env \
  ant2oa
```

#### 5. Using Docker Compose (Recommended)

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  ant2oa:
    build: .
    ports:
      - "8080:8080"
    environment:
      - OPENAI_BASE_URL=https://api.deepseek.com/v1
      - OPENAI_MODEL=deepseek-chat
      - LISTEN_ADDR=:8080
      - RATE_LIMIT=200
    restart: unless-stopped
    volumes:
      - ./logs:/app/logs
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

Run:

```bash
docker-compose up -d
```

## 🔧 Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `OPENAI_BASE_URL` | ✅ | - | Base URL of OpenAI-compatible service |
| `OPENAI_MODEL` | ✅ | - | Name of the model to use |
| `LISTEN_ADDR` | ❌ | `:8080` | Service listening address and port |
| `RATE_LIMIT` | ❌ | Unlimited | Requests per minute limit (0 means unlimited) |

### Common Configuration Examples

#### DeepSeek Configuration
```bash
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat
```

#### Ollama Local Configuration
```bash
OPENAI_BASE_URL=http://localhost:11434/v1
OPENAI_MODEL=llama3.1:8b
LISTEN_ADDR=:8080
```

#### vLLM Configuration
```bash
OPENAI_BASE_URL=http://your-vllm-server:8000/v1
OPENAI_MODEL=your-model-name
```

## 📡 API Endpoints

The service provides the following API endpoints:

- `POST /v1/messages` - Send messages (main endpoint)
- `POST /v1/complete` - Text completion
- `GET /v1/models` - Get available models list
- `GET /health` - Health check

### Usage Examples

#### curl Testing

```bash
# Health check
curl http://localhost:8080/health

# Send message
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "max_tokens": 1000,
    "messages": [
      {"role": "user", "content": "Hello, please introduce yourself"}
    ]
  }'
```

#### JavaScript/TypeScript

```javascript
const response = await fetch('http://localhost:8080/v1/messages', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    model: 'deepseek-chat',
    max_tokens: 1000,
    messages: [
      { role: 'user', content: 'Hello, please introduce yourself' }
    ]
  })
});

const data = await response.json();
console.log(data);
```

## 🔍 Troubleshooting

### Common Issues

1. **Service Won't Start**
   - Check if environment variables are correctly set
   - Confirm the port is not in use
   - Check log messages

2. **Request Failed**
   - Verify `OPENAI_BASE_URL` is accessible
   - Confirm API key configuration is correct
   - Check network connectivity

3. **Model Not Supported**
   - Confirm `OPENAI_MODEL` is a supported model of the target service
   - Check model name spelling

### Debug Mode

Set log level and view detailed logs:

```bash
# View system service logs
sudo journalctl -u ant2oa -f

# Run directly to view logs
LOG_LEVEL=debug ./ant2oa
```

## 🏗️ Project Structure

```
ant2oa/
├── main.go          # Main program entry point
├── api.go          # API handling logic
├── proxy.go        # Proxy forwarding logic
├── types.go        # Data type definitions
├── utils.go        # Utility functions
├── install.go      # Service installation script
├── go.mod          # Go module definition
├── .env            # Environment configuration (optional)
└── README.md       # Project documentation
```

## 📈 Performance Features

- 🚀 **High Performance**: Go-based high-concurrency processing
- ⚡ **Low Latency**: Lightweight proxy design
- 🛡️ **Traffic Control**: Configurable request rate limiting
- 🔄 **Load Balancing**: Support for multi-instance deployment
- 📊 **Health Checks**: Built-in health monitoring endpoint
- 🏗️ **Cross-Platform**: Support for x86_64, ARM64, and other architectures

## 🤝 Contributing

Issues and Pull Requests are welcome!

## 📄 License

This project uses the GNU GPL v3 License. See [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

Thanks to all developers and organizations contributing to the large language model ecosystem.

---

<div align="center">
  <p>⭐ If this project is helpful to you, please give it a Star!</p>
  <p>Built with ❤️ using Go</p>
</div>