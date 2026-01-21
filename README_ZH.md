# ant2oa

> 将 OpenAI 兼容 API 转换为 Anthropic API 格式的高性能代理服务

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/License-MIT-green.svg?style=flat)](LICENSE)

🌐 **Language / 语言**: [English](README.md) | [中文](README_ZH.md)

## 🎯 项目简介

`ant2oa` 是一个高性能的 Go 语言代理服务，主要功能是将 **OpenAI 兼容的 API** 转换成 **Anthropic (Claude) API** 格式。

### 核心价值

这个工具的核心作用是"协议转换"：让那些原本只支持 Anthropic API 的客户端和应用，能够无缝调用各种 OpenAI 兼容的大语言模型服务。

#### 支持的 OpenAI 兼容服务
- 🧠 **DeepSeek** (V3/R1 等最新模型)
- 🏢 **OpenAI** (GPT-4, GPT-3.5 等)
- 🚀 **vLLM** (自部署大模型服务)
- 🦙 **Ollama** (本地大模型推理)
- 📡 **其他 OpenAI 兼容 API 服务**

#### 适配的 Claude 客户端
- 💻 **Cursor** (AI 代码编辑器)
- 🤖 **Cline** (AI 编程助手)
- 🛠️ **各类 Agent 工具**
- 📝 **Claude 原生客户端**

## 🚀 快速开始

### 方式一：直接运行（推荐用于测试）

1. **下载并运行**

```bash
# 克隆项目
git clone <your-repo-url>
cd ant2oa

# 运行服务
go run .
```

2. **配置环境变量**

创建 `.env` 文件：

```bash
# 必需配置
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat

# 可选配置
LISTEN_ADDR=:8080
RATE_LIMIT=100
```

3. **测试服务**

```bash
curl http://localhost:8080/health
```

### 方式二：生产环境部署

#### 1. 构建可执行文件

```bash
# 构建
go build -o ant2oa .

# 或使用交叉编译构建不同平台版本
GOOS=linux GOARCH=amd64 go build -o ant2oa-linux-amd64 .
GOOS=darwin GOARCH=amd64 go build -o ant2oa-darwin-amd64 .
GOOS=windows GOARCH=amd64 go build -o ant2oa-windows-amd64.exe .

# ARM64 架构支持（树莓派、ARM服务器等）
GOOS=linux GOARCH=arm64 go build -o ant2oa-linux-arm64 .
```

#### 2. 配置服务

创建 `env` 或 `.env` 配置文件：

```bash
# OpenAI 兼容服务配置
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat

# 服务器配置
LISTEN_ADDR=:8080
PORT=8080

# 性能配置
RATE_LIMIT=200  # 每分钟请求数限制，0 表示无限制

# 日志级别
LOG_LEVEL=info
```

#### 3. Linux 系统服务安装（生产环境推荐）

```bash
# 以管理员权限运行安装命令
sudo ./ant2oa -install

# 服务管理命令
sudo systemctl start ant2oa    # 启动服务
sudo systemctl stop ant2oa     # 停止服务
sudo systemctl restart ant2oa  # 重启服务
sudo systemctl status ant2oa   # 查看服务状态

# 查看日志
journalctl -u ant2oa -f        # 实时查看日志
journalctl -u ant2oa --since "2024-01-01" # 查看历史日志
```

#### 4. Docker 部署

创建 `Dockerfile`：

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

构建和运行：

```bash
# 构建镜像
docker build -t ant2oa .

# 运行容器
docker run -d \
  --name ant2oa \
  -p 8080:8080 \
  --env-file env \
  ant2oa
```

#### 5. 使用 Docker Compose（推荐）

创建 `docker-compose.yml`：

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

运行：

```bash
docker-compose up -d
```

## 🔧 配置说明

### Web UI 配置

在浏览器中打开 `http://localhost:8080/config` 访问 Web 配置界面。
**需要身份验证** - 默认密码为 `admin`（可通过 `ADMIN_PASSWORD` 环境变量修改）。

Web UI 允许您：
- 通过简单表单配置服务设置
- 设置监听地址、OpenAI 服务 URL、模型名称和速率限制
- 配置自动保存到 `.env` 文件

```bash
# 访问配置页面（浏览器会提示输入密码）
http://localhost:8080/config

# 或使用 curl 进行配置操作（需要认证）
curl -u :admin http://localhost:8080/api/config
```

### 配置 API

```bash
# 获取当前配置（需要认证）
curl -u :admin http://localhost:8080/api/config

# 更新配置
curl -u :admin -X POST http://localhost:8080/api/config \
  -H "Content-Type: application/json" \
  -d '{
    "listenAddr": ":8080",
    "baseUrl": "https://api.deepseek.com/v1",
    "model": "deepseek-chat",
    "rateLimit": "100"
  }'
```

### 环境变量

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `OPENAI_BASE_URL` | ✅ | - | OpenAI 兼容服务的基础 URL |
| `OPENAI_MODEL` | ❌ | - | 默认模型名称（请求中未指定模型时使用）。API 请求中的 `model` 参数会透传给上游服务。 |
| `LISTEN_ADDR` | ❌ | `:8080` | 服务监听地址和端口 |
| `RATE_LIMIT` | ❌ | 无限制 | 每分钟请求数限制（0 表示无限制） |
| `ADMIN_PASSWORD` | ❌ | `admin` | 配置页面访问密码 |

### 常用配置示例

#### DeepSeek 配置
```bash
OPENAI_BASE_URL=https://api.deepseek.com/v1
OPENAI_MODEL=deepseek-chat  # 可选：作为默认模型
```

#### Ollama 本地配置
```bash
OPENAI_BASE_URL=http://localhost:11434/v1
OPENAI_MODEL=llama3.1:8b  # 可选：作为默认模型
LISTEN_ADDR=:8080
```

#### vLLM 配置
```bash
OPENAI_BASE_URL=http://your-vllm-server:8000/v1
OPENAI_MODEL=your-model-name  # 可选：作为默认模型
```

## 📡 API 端点

服务提供以下 API 端点：

- `GET /config` - Web 配置界面（需要管理员认证）
- `GET/POST /api/config` - 配置管理 API（需要管理员认证）
- `POST /v1/messages` - 发送消息（主要端点）
- `POST /v1/complete` - 文本补全
- `GET /v1/models` - 获取可用模型列表
- `GET /health` - 健康检查

### 使用示例

#### curl 测试

```bash
# 健康检查
curl http://localhost:8080/health

# 发送消息
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "max_tokens": 1000,
    "messages": [
      {"role": "user", "content": "你好，请介绍一下自己"}
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
      { role: 'user', content: '你好，请介绍一下自己' }
    ]
  })
});

const data = await response.json();
console.log(data);
```

## 🔍 故障排除

### 常见问题

1. **服务无法启动**
   - 检查环境变量是否正确设置
   - 确认端口未被占用
   - 查看日志信息

2. **请求失败**
   - 验证 `OPENAI_BASE_URL` 是否可访问
   - 确认 API 密钥配置正确
   - 检查网络连接

3. **模型不支持**
   - 确认 `OPENAI_MODEL` 是目标服务支持的模型
   - 检查模型名称拼写

### 调试模式

设置日志级别并查看详细日志：

```bash
# 查看系统服务日志
sudo journalctl -u ant2oa -f

# 直接运行查看日志
LOG_LEVEL=debug ./ant2oa
```

## 🏗️ 项目结构

```
ant2oa/
├── main.go          # 主程序入口
├── api.go          # API 处理逻辑
├── proxy.go        # 代理转发逻辑
├── types.go        # 数据类型定义
├── utils.go        # 工具函数
├── install.go      # 服务安装脚本
├── go.mod          # Go 模块定义
├── .env            # 环境配置（可选）
└── README.md       # 项目文档
```

## 📈 性能特性

- 🚀 **高性能**：基于 Go 语言的高并发处理
- ⚡ **低延迟**：轻量级代理设计
- 🛡️ **流量控制**：可配置请求限流
- 🔄 **负载均衡**：支持多实例部署
- 📊 **健康检查**：内置健康监控端点
- 🏗️ **跨平台支持**：支持 x86_64、ARM64 等多种架构

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

本项目采用 MIT 许可证。详情请查看 [LICENSE](LICENSE) 文件。

## 🙏 致谢

感谢所有为大语言模型生态做出贡献的开发者和组织。

---

<div align="center">
  <p>⭐ 如果这个项目对您有帮助，请给个 Star 支持一下！</p>
  <p>Built with ❤️ using Go</p>
</div>
