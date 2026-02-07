# Gemini Web Proxy - 项目需求文档 (REQUIREMENT.md)

## 1. 项目概述

本项目名为 **Gemini-Web-Proxy**。这是一个通过"浏览器插件"操控"Gemini 网页端"的中间件系统。
它允许用户通过兼容 OpenAI 格式的本地 RESTful API 接口，间接与 Google Gemini (https://gemini.google.com) 进行对话，利用浏览器已登录的 Session 绕过复杂的逆向工程。

### 系统架构

系统由两部分组成，通过 **WebSocket** 进行双向通讯：

1. **Server (服务端)**: Golang 本地服务器。负责提供 OpenAI 兼容 REST API、管理数据 (SQLite)、维持与插件的 WS 连接。
2. **Extension (插件端)**: Chrome 浏览器插件 (Manifest V3)。负责在后台维持连接、注入脚本到 Gemini 页面、模拟用户操作 (输入/点击)、监听并回传 DOM 变化。

---

## 2. 技术栈规范

### 2.1 服务端 (Server)

* **语言**: Golang 1.22+
* **Web 框架**: Gin (`github.com/gin-gonic/gin`)
* **WebSocket**: Gorilla WebSocket (`github.com/gorilla/websocket`)
* **ORM**: GORM (`gorm.io/gorm`)
* **数据库**: SQLite (`github.com/mattn/go-sqlite3`)
* **配置管理**: YAML (`gopkg.in/yaml.v3`)，配置文件为项目根目录 `config.yaml`
* **功能职责**:
  * 启动时读取 `config.yaml` 配置文件。
  * 启动 HTTP Server 监听配置端口（默认 `:8080`）。
  * 提供 `/ws` 路由供插件连接。
  * 提供 `/v1/chat/completions` 路由供外部调用（兼容 OpenAI API 格式）。
  * 支持 SSE 流式返回 (`stream: true`) 和非流式返回。
  * 持久化存储对话历史。

### 2.3 配置文件 (config.yaml)

```yaml
server:
  port: 8080               # HTTP 服务端口

database:
  path: "./data.db"         # SQLite 数据库文件路径

websocket:
  ping_interval: 30         # 心跳间隔 (秒)
  pong_timeout: 10          # 等待 PONG 超时 (秒)
```

### 2.2 插件端 (Extension)

* **规范**: Manifest V3
* **语言**: TypeScript
* **构建工具**: Vite (使用 `@crxjs/vite-plugin` 或类似插件)
* **核心模块**:
  * `background.ts`: Service Worker，负责 WS 连接保活、心跳、任务分发。
  * `content.ts`: 注入 `gemini.google.com`，负责 DOM 操作 (输入 prompt) 和 MutationObserver (监听回复)。

---

## 3. API 设计 (兼容 OpenAI 格式)

### 3.1 `POST /v1/chat/completions`

**请求体**（兼容 OpenAI）:
```json
{
  "model": "gemini",
  "messages": [
    {"role": "user", "content": "你好"}
  ],
  "stream": false
}
```

**非流式响应** (`stream: false`):
```json
{
  "id": "chatcmpl-uuid",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "gemini",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Gemini 的回复内容..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  }
}
```

**流式响应** (`stream: true`), SSE 格式:
```
data: {"id":"chatcmpl-uuid","object":"chat.completion.chunk","created":1700000000,"model":"gemini","choices":[{"index":0,"delta":{"role":"assistant","content":"你"},"finish_reason":null}]}

data: {"id":"chatcmpl-uuid","object":"chat.completion.chunk","created":1700000000,"model":"gemini","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null}]}

data: {"id":"chatcmpl-uuid","object":"chat.completion.chunk","created":1700000000,"model":"gemini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

> 注意：由于 Gemini 网页端无法精确统计 token，`usage` 字段全部返回 0。

---

## 4. 通讯协议 (WebSocket Protocol)

通讯格式统一使用 JSON。

### 4.1 服务端 -> 插件 (指令)

```json
{
  "id": "uuid-v4-string",
  "type": "CMD_SEND_MESSAGE",
  "payload": {
    "prompt": "Hello Gemini",
    "conversation_id": ""
  }
}
```

### 4.2 插件 -> 服务端 (事件/回复)

```json
{
  "reply_to": "uuid-v4-string",
  "type": "EVENT_REPLY",
  "payload": {
    "text": "Gemini 的回复内容...",
    "status": "DONE",
    "conversation_id": "..."
  }
}
```

事件类型: `EVENT_REPLY` | `EVENT_ERROR` | `EVENT_PONG`
状态: `DONE` (完成) | `PROCESSING` (生成中)

---

## 5. 数据库设计 (SQLite Schema)

GORM 模型：

1. **Conversation (对话)**
   * `ID` (string, primary key): Gemini 网页 URL 中的 hash ID。
   * `Title` (string): 对话标题。
   * `CreatedAt` (datetime)。

2. **Message (消息)**
   * `ID` (uint, auto increment)。
   * `ConversationID` (string, foreign key)。
   * `Role` (string): "user" 或 "model"。
   * `Content` (text): 消息内容。
   * `Status` (string): "pending", "sent", "received", "error"。

---

## 6. 详细功能逻辑

### 6.1 连接与保活 (Heartbeat)

* **Extension (Background)**: 启动时连接 `ws://localhost:8080/ws`。
* **Server**: 每 30 秒发送 `{"type": "PING"}`。
* **Extension**: 收到 PING 后立即回复 `{"type": "PONG"}`。
* **断线重连**: Extension 需监听 `onclose`，若断开则每 5 秒尝试重连。

### 6.2 发送消息流程 (核心)

1. **用户** 调用 Server API: `POST /v1/chat/completions`。
2. **Server** 提取 messages 中最后一条 user 消息作为 prompt，生成 UUID，将消息存入 DB (Status=pending)，通过 WS 发送 `CMD_SEND_MESSAGE` 给插件。
3. **Extension (Background)** 收到指令：
   * 检查是否存在 `gemini.google.com` 的 Tab。
   * 如果不存在，创建新 Tab 并等待加载完成。
   * 如果存在，激活该 Tab。
   * 通过 `chrome.tabs.sendMessage` 将指令转发给 **Content Script**。
4. **Extension (Content Script)** 执行 DOM 操作：
   * **定位输入框**: 寻找 `div[contenteditable="true"]` 或 `rich-textarea`。
   * **模拟输入**: 触发 `input` 事件，使用 `document.execCommand('insertText')` 写入。
   * **点击发送**: 寻找发送按钮并点击。

### 6.3 接收回复流程 (核心)

1. **Extension (Content Script)** 在点击发送后：
   * 使用 `MutationObserver` 监听对话列表容器的变化。
   * **判断生成状态**: 检测"Stop responding"按钮（正在生成）或"Regenerate"图标（生成结束）。
   * **提取内容**: 获取最后一个 `model-response` 容器的文本/HTML。
2. **Extension** 将提取的内容通过 WS 发回 Server (`EVENT_REPLY`)。
   * `status: "PROCESSING"` — 生成中，附带当前已生成的文本。
   * `status: "DONE"` — 生成完成，附带完整文本。
3. **Server** 收到回复后：
   * 如果请求是流式 (`stream: true`)：将 PROCESSING 和 DONE 事件实时转换为 SSE chunk 推送给客户端。
   * 如果请求是非流式 (`stream: false`)：等待 DONE 状态后，一次性返回完整响应。
   * 更新 DB 中的消息内容和状态。

---

## 7. 关键难点与应对策略

1. **DOM 选择器**: Google 的 Class Name 可能是动态混淆的。**策略**: 优先使用 ARIA 属性 (`aria-label`, `role="button"`) 或相对路径，避免硬编码随机类名。

2. **React 输入框**: 直接修改 `.innerHTML` 不会生效。**策略**: 使用 `document.execCommand('insertText')` 或手动分发 `new Event('input', { bubbles: true })`。

3. **多 Tab 冲突**: 插件应强制只在一个 Gemini Tab 中工作。如果有多个，选取最近活动的一个。

---

## 8. 开发步骤

1. **Step 1**: 初始化 Golang Server，实现 WS 连接、心跳处理和基本的 GORM SQLite 结构。
2. **Step 2**: 初始化 Chrome Extension (Vite + TS)，配置 Manifest V3，实现 `background.ts` 连接 Server 并打印日志。
3. **Step 3**: 实现 Server 的 OpenAI 兼容 API 接口 (`POST /v1/chat/completions`)，支持流式与非流式，以及 WS 消息广播。
4. **Step 4 (最重要)**: 编写 `content.ts` 的 DOM 操作逻辑，实现精准的输入框定位、模拟输入和回复监听。
