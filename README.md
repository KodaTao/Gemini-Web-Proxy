# Gemini Web Proxy

<p align="center">
  <strong>通过 OpenAI 兼容 API 使用 Google Gemini 网页版</strong>
</p>

<p align="center">
  <a href="#特性">特性</a> &bull;
  <a href="#工作原理">工作原理</a> &bull;
  <a href="#快速开始">快速开始</a> &bull;
  <a href="#api-使用">API 使用</a> &bull;
  <a href="#配置">配置</a> &bull;
  <a href="#常见问题">FAQ</a>
</p>

---

**Gemini Web Proxy** 是一个通过 Chrome 插件操控 [Google Gemini](https://gemini.google.com) 网页端的中间件系统，对外暴露 **OpenAI 兼容的 REST API**。

你可以直接用任何支持 OpenAI API 的工具（如 [Cursor](https://cursor.com)、[ChatBox](https://chatboxai.app/)、[Open WebUI](https://openwebui.com/) 等）来调用 Gemini，无需 API Key，利用浏览器已登录的 Session 即可。

## 特性

- **OpenAI 兼容 API** — 支持 `POST /v1/chat/completions`，流式 (SSE) 和非流式响应
- **多角色对话** — 完整支持 `system`、`user`、`assistant` 角色，以 XML 格式传递对话上下文
- **自动选择 Pro 模型** — 每次对话自动创建新会话并切换到 Gemini Pro
- **反检测优化** — 剪贴板粘贴输入、完整鼠标事件链、随机化操作延时
- **并发控制** — Server 端信号量 + 插件端状态同步，自动拒绝并发请求 (429)
- **自动清理** — 每次对话完成后自动删除历史，保持浏览器端整洁
- **WebSocket 保活** — 应用层心跳机制，插件断线自动重连

## 工作原理

```
┌──────────────┐     HTTP/SSE      ┌──────────────┐    WebSocket     ┌──────────────────┐
│  Any OpenAI  │ ───────────────── │    Server     │ ─────────────── │ Chrome Extension │
│  Compatible  │  /v1/chat/        │   (Golang)    │   双向通讯       │  (Manifest V3)   │
│    Client    │  completions      │  :6543        │                 │                  │
└──────────────┘                   └──────────────┘                 └────────┬─────────┘
                                                                            │ DOM 操作
                                                                   ┌────────▼─────────┐
                                                                   │  gemini.google.com│
                                                                   │   (浏览器网页)     │
                                                                   └──────────────────┘
```

1. 客户端发送 OpenAI 格式请求到 Server
2. Server 通过 WebSocket 将指令下发给 Chrome 插件
3. 插件在 Gemini 网页上模拟人类操作（输入、点击、等待回复）
4. 插件提取回复内容通过 WebSocket 回传给 Server
5. Server 将回复转换为 OpenAI 格式返回给客户端

## 快速开始

### 前置要求

- Chrome 浏览器，并已登录 [Google Gemini](https://gemini.google.com)
- Node.js 18+ (用于构建插件)

### 1. 下载 Server

从 [Releases](https://github.com/KodaTao/Gemini-Web-Proxy/releases) 页面下载对应平台的二进制文件：

| 平台 | 文件名 |
|------|--------|
| macOS (Apple Silicon) | `gemini-web-proxy-darwin-arm64` |
| macOS (Intel) | `gemini-web-proxy-darwin-amd64` |
| Linux (x86_64) | `gemini-web-proxy-linux-amd64` |
| Windows (x86_64) | `gemini-web-proxy-windows-amd64.exe` |

> 或者从源码编译：参见 [从源码构建](#从源码构建)

### 2. 启动 Server

**最简启动**（使用默认配置，无需 config.yaml）：

```bash
# macOS / Linux
chmod +x gemini-web-proxy-*
./gemini-web-proxy-darwin-arm64

# Windows
gemini-web-proxy-windows-amd64.exe
```

**指定配置文件**：

```bash
./gemini-web-proxy-darwin-arm64 -c ./config.yaml
```

**设置 API Key**（公网部署时推荐）：

```bash
./gemini-web-proxy-darwin-arm64 -api-key your-secret-key
```

> 设置 API Key 后，客户端需在请求头中携带 `Authorization: Bearer your-secret-key`。

### 3. 构建并安装 Chrome 插件

```bash
cd extension
npm install
npm run build
```

在 Chrome 中加载插件：

1. 打开 `chrome://extensions/`
2. 开启右上角 **开发者模式**
3. 点击 **加载已解压的扩展程序**
4. 选择 `extension/dist` 目录

### 4. 连接插件

1. 打开 [gemini.google.com](https://gemini.google.com) 并保持页面打开
2. 点击插件图标，配置 WebSocket 地址为 `ws://localhost:6543/ws`
3. 点击 **保存配置**
4. 页面右下角出现绿色状态指示灯即表示连接成功

### 5. 开始使用

使用任何 OpenAI 兼容客户端，将 API Base URL 指向 `http://localhost:6543/v1`：

```bash
# 快速测试
curl http://localhost:6543/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

## API 使用

### 非流式请求

```bash
curl http://localhost:6543/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini",
    "messages": [
      {"role": "user", "content": "What is 1+1?"}
    ],
    "stream": false
  }'
```

### 流式请求 (SSE)

```bash
curl http://localhost:6543/v1/chat/completions \
  -H "Content-Type: application/json" \
  --no-buffer \
  -d '{
    "model": "gemini",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Tell me a joke."}
    ],
    "stream": true
  }'
```

### 在第三方工具中使用

| 工具 | API Base URL | API Key |
|------|-------------|---------|
| Cursor | `http://localhost:6543/v1` | 任意值 (如 `sk-xxx`) |
| ChatBox | `http://localhost:6543/v1` | 任意值 |
| Open WebUI | `http://localhost:6543/v1` | 任意值 |

> 如果未设置 `-api-key`，Server 不验证 API Key，填写任意非空值即可。
> 如果设置了 `-api-key`，则需填写对应的 Key 值。

### Python OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:6543/v1",
    api_key="any-key",
)

response = client.chat.completions.create(
    model="gemini",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"},
    ],
)
print(response.choices[0].message.content)
```

### 错误码

| 状态码 | 含义 |
|--------|------|
| 200 | 成功 |
| 400 | 请求格式错误或缺少 user 消息 |
| 401 | API Key 验证失败 |
| 429 | 插件正忙或 Server 正在处理其他请求 |
| 503 | 插件未连接 |

## 配置

### 命令行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-c <path>` | 指定 config.yaml 文件路径 | 不指定则使用默认配置 |
| `-api-key <key>` | 设置 API Key（优先级高于配置文件） | 空（不验证） |

### config.yaml

```yaml
server:
  port: 6543               # HTTP 服务端口
  mode: "release"          # debug/test/release

database:
  path: "./data.db"         # SQLite 数据库文件路径

websocket:
  ping_interval: 30         # 心跳间隔 (秒)
  pong_timeout: 10          # 等待 PONG 超时 (秒)

api_key: ""                 # API Key，为空则不验证
```

> 不指定 `-c` 参数时，Server 使用内置默认配置运行，启动时会打印生效的配置信息。

### 插件配置

点击 Chrome 工具栏中的插件图标，可以配置：

- **WebSocket 地址** — 默认 `ws://localhost:6543/ws`

## 从源码构建

### 构建 Server

```bash
# 需要 Go 1.22+
cd server
go build -o gemini-web-proxy .
```

### 构建插件

```bash
cd extension
npm install
npm run build
```

### 交叉编译（所有平台）

```bash
# 需要 macOS + Homebrew
# 首次运行需安装交叉编译工具链
./build.sh
```

编译产物在 `release/` 目录下。

## 注意事项

- **请保持 Gemini 网页处于打开状态**，插件需要在页面上执行 DOM 操作
- **不支持并发请求**，同一时间只能处理一个对话，后续请求会收到 429 错误
- **每次对话后会自动删除**，不会在 Gemini 网页端留下历史记录
- 本项目仅供学习和个人使用，请遵守 Google 的服务条款

## 项目结构

```
Gemini-Web-Proxy/
├── server/                 # Golang 后端
│   ├── main.go             # 入口
│   ├── config/             # 配置加载
│   ├── handler/            # WebSocket + API 处理
│   └── model/              # 数据库模型
├── extension/              # Chrome 插件 (MV3 + TypeScript)
│   ├── src/
│   │   ├── background.ts   # Service Worker (WS 连接)
│   │   ├── content.ts      # DOM 操作 (核心逻辑)
│   │   ├── overlay.ts      # 状态悬浮窗
│   │   └── popup.ts        # 配置页面
│   └── dist/               # 构建产物
├── config.yaml             # Server 配置文件
└── build.sh                # 交叉编译脚本
```

## 常见问题

<details>
<summary><b>插件显示"未连接"怎么办？</b></summary>

1. 确认 Server 已启动
2. 检查插件配置的 WebSocket 地址是否正确（默认 `ws://localhost:6543/ws`）
3. 打开 Chrome 开发者工具 → 控制台，查看是否有连接错误
</details>

<details>
<summary><b>请求返回 429 错误</b></summary>

插件正忙，等上一个请求完成后再试。每次请求需要约 10-30 秒（取决于 Gemini 的响应速度）。
</details>

<details>
<summary><b>请求返回 503 错误</b></summary>

插件未连接。请确保：
- Gemini 网页已打开 (`gemini.google.com`)
- 插件已加载并启用
- 页面右下角有绿色状态指示灯
</details>

<details>
<summary><b>可以部署在服务器上吗？</b></summary>

Server 可以部署在任何机器上，但 Chrome 插件必须运行在有浏览器的环境中。你可以在服务器上运行 Server，在本地电脑上运行插件，将插件的 WebSocket 地址指向服务器 IP。
</details>

## License

[Apache License 2.0](LICENSE)
