# Gemini Web Proxy - 项目需求文档 (REQUIREMENT.md)

## 1. 项目概述

本项目名为 **Gemini-Web-Proxy**。这是一个通过"浏览器插件"操控"Gemini 网页端"的中间件系统。
它允许用户通过兼容 OpenAI 格式的本地 RESTful API 接口，间接与 Google Gemini (https://gemini.google.com) 进行对话，利用浏览器已登录的 Session 绕过复杂的逆向工程。

### 系统架构

系统由两部分组成，通过 **WebSocket** 进行双向通讯：

1. **Server (服务端)**: Golang 本地服务器。负责提供 OpenAI 兼容 REST API、管理数据 (SQLite)、维持与插件的 WS 连接。
2. **Extension (插件端)**: 浏览器插件 (Manifest V3)，同时支持 **Chrome** 和 **Safari**。负责在后台维持连接、注入脚本到 Gemini 页面、模拟用户操作 (输入/点击)、监听并回传 DOM 变化。

---

## 2. 技术栈规范

### 2.1 服务端 (Server)

* **语言**: Golang 1.22+
* **Web 框架**: Gin (`github.com/gin-gonic/gin`)
* **WebSocket**: Gorilla WebSocket (`github.com/gorilla/websocket`)
* **ORM**: GORM (`gorm.io/gorm`)
* **数据库**: SQLite (`github.com/mattn/go-sqlite3`)
* **配置管理**: YAML (`gopkg.in/yaml.v3`)，支持 `-c` 参数指定配置文件路径，不指定则使用内置默认配置
* **功能职责**:
  * 启动时通过 `-c` 参数读取配置文件，未指定则使用默认配置并打印到控制台。
  * 支持 `-api-key` 参数设置 API Key，部署到公网时保证安全性。
  * 启动 HTTP Server 监听配置端口（默认 `:6543`）。
  * 提供 `/ws` 路由供插件连接。
  * 提供 `/v1/chat/completions` 路由供外部调用（兼容 OpenAI API 格式）。
  * 支持 SSE 流式返回 (`stream: true`) 和非流式返回。
  * 持久化存储对话历史。

### 2.3 配置文件 (config.yaml)

```yaml
server:
  port: 6543               # HTTP 服务端口

database:
  path: "./data.db"         # SQLite 数据库文件路径

websocket:
  ping_interval: 30         # 心跳间隔 (秒)
  pong_timeout: 10          # 等待 PONG 超时 (秒)

api_key: ""                 # API Key，为空则不验证
```

**命令行参数**:
* `-c <path>`: 指定 config.yaml 文件路径，不指定则使用默认配置
* `-api-key <key>`: 设置 API Key（优先级高于配置文件中的 api_key）

### 2.2 插件端 (Extension)

* **规范**: Manifest V3
* **语言**: TypeScript
* **构建工具**: Vite (library 模式分别打包各入口文件)
* **配置存储**: `chrome.storage.local`（Safari 中同样支持 `chrome.*` 命名空间）
* **浏览器支持**:
  * **Chrome**: 直接加载 `extension/dist` 目录
  * **Safari** (macOS): 通过 `xcrun safari-web-extension-converter` 转换为 Xcode 项目，使用开发者模式加载（无需 Apple Developer 账号和签名）
* **核心模块**:
  * `background.ts`: Service Worker，负责 WS 连接保活、心跳、任务分发。从 storage 读取 WS 地址，配置变更时自动重连。
  * `content.ts`: 注入 `gemini.google.com`，负责 DOM 操作 (输入 prompt) 和 MutationObserver (监听回复)。同时在页面右下角注入悬浮窗，实时显示 WS 连接状态和当前任务状态。
  * `overlay.ts`: 悬浮窗模块，使用 Shadow DOM 隔离样式，支持拖拽移动，显示连接状态（绿/红点）和任务状态（空闲/处理中/完成）。内嵌设置面板，点击⚙按钮可展开配置 WebSocket 地址，保存后自动触发重连。
  * `popup.html/ts/css`: 插件配置页面，可配置 WebSocket 地址（默认 `ws://localhost:8080/ws`），使用 `chrome.storage.local` 持久化。

### 2.4 Safari Web Extension 支持

* **系统要求**: macOS 14.0+ 配合 Safari 17+，或 macOS 12.0+ 配合 Safari 16.4+（需支持 Service Worker Background）
* **开发工具**: Xcode 15+（用于转换和构建，但不需要 Apple Developer 账号）
* **构建方式**:
  1. 先执行 `npm run build` 构建 Chrome 扩展到 `dist/`
  2. 执行 `npm run build:safari` 调用 `xcrun safari-web-extension-converter` 将 `dist/` 转换为 Xcode 项目
  3. 在 Xcode 中构建并运行，或通过命令行 `xcodebuild` 构建
* **运行方式**（无需签名）:
  1. Safari → 设置 → 高级 → 勾选"在菜单栏中显示开发菜单"
  2. 开发 → 勾选"允许未签名的扩展"
  3. Safari → 设置 → 扩展 → 启用 Gemini Web Proxy
* **兼容性说明**:
  * Safari Web Extension 支持 `chrome.*` 命名空间，插件源码无需为 Safari 做特殊修改
  * `navigator.clipboard.readText()` 在 Safari 中需要用户授予剪贴板权限，已有 fallback 处理
  * 快捷键 `metaKey` (Cmd) 在 macOS Safari 中与 Chrome 行为一致

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

### 4.3 插件 -> 服务端 (状态上报)

```json
{
  "type": "EVENT_STATUS",
  "payload": {
    "status": "idle"
  }
}
```

状态值: `idle` (空闲，可接收新任务) | `busy` (忙碌，正在处理任务)

* 插件连接后默认为 `idle` 状态。
* 插件在开始处理 `CMD_SEND_MESSAGE` 时发送 `busy`。
* 插件完成所有操作（包括删除对话）后发送 `idle`。
* Server 收到 API 请求时，先检查插件状态，若为 `busy` 则直接返回 429 错误。

事件类型: `EVENT_REPLY` | `EVENT_ERROR` | `EVENT_PONG` | `EVENT_STATUS`
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

* **Extension (Background)**: 启动时连接 `ws://localhost:6543/ws`。
* **Server**: 每 30 秒发送 `{"type": "PING"}`。
* **Extension**: 收到 PING 后立即回复 `{"type": "PONG"}`。
* **断线重连**: Extension 需监听 `onclose`，若断开则每 5 秒尝试重连。

### 6.2 发送消息流程 (核心)

1. **用户** 调用 Server API: `POST /v1/chat/completions`。
2. **Server** 先检查并发限制（信号量）和插件端状态（`extensionReady`），若插件忙碌则返回 429 错误。通过后将所有 messages（system/user/assistant）序列化为 XML 格式作为 prompt，生成 UUID，将消息存入 DB (Status=pending)，通过 WS 发送 `CMD_SEND_MESSAGE` 给插件。
   * **XML 格式**: 使用 `<chat_history>` 根标签，每条消息为 `<message role="..."><Content><![CDATA[...]]></Content></message>`，完整保留对话上下文。
3. **Extension (Background)** 收到指令：
   * 检查是否存在 `gemini.google.com` 的 Tab。
   * 如果不存在，创建新 Tab 并等待加载完成。
   * 如果存在，激活该 Tab。
   * 通过 `chrome.tabs.sendMessage` 将指令转发给 **Content Script**。
4. **Extension (Content Script)** 执行 DOM 操作：
   * **新对话处理**: 如果请求未携带 `conversation_id`，先通过 `Shift+Cmd+O` 快捷键创建新对话，并检查确保使用 Pro 模型。
   * **定位输入框**: 寻找 Quill 编辑器 `rich-textarea .ql-editor[contenteditable="true"]`（新版 UI 使用 `rich-textarea` 组件包裹，class 含 `new-input-ui`，`aria-label="在此处输入提示"`）。
   * **模拟输入**: 多策略尝试 (剪贴板粘贴 → execCommand → 直接 DOM 操作)。输入内容为 XML 格式的完整对话历史，包含 system/user/assistant 角色。
   * **点击发送**: 等待发送按钮从 `aria-disabled` 变为可用后点击（含重试机制）。

### 6.3 接收回复流程 (核心)

1. **Extension (Content Script)** 在点击发送后：
   * 使用 `MutationObserver` 监听对话列表容器的变化。
   * **判断生成状态**: 检测"Stop responding"按钮（正在生成）或"Regenerate"图标（生成结束）。
   * **提取内容**: 获取最后一个 `model-response` 容器的文本/HTML。
2. **Extension** 将提取的内容通过 WS 发回 Server (`EVENT_REPLY`)。
   * `status: "PROCESSING"` — 生成中，附带当前已生成的纯文本（DOM 提取）。
   * `status: "DONE"` — 生成完成，附带 **Markdown 格式**的完整文本（通过 cheerio + turndown 将回复 HTML 转换为 Markdown）。
   * 回复内容提取时过滤思考过程区域 (`.model-thoughts`)，优先从 `.markdown-main-panel` 元素获取 HTML。
   * **回复完成后的流程**：HTML→Markdown 转换 → 删除当前对话 → 发送 DONE → 上报 idle。
3. **Server** 收到回复后：
   * 如果请求是流式 (`stream: true`)：PROCESSING 事件计算差量推送 SSE chunk（纯文本），DONE 时不追加内容 delta（避免与 Markdown 文本重复），只发 finish chunk。
   * 如果请求是非流式 (`stream: false`)：等待 DONE 状态后，一次性返回 Markdown 格式的完整响应。
   * 更新 DB 中的消息内容和状态（数据库存储 Markdown 格式）。

---

## 7. 关键难点与应对策略

1. **DOM 选择器**: Google 的 Class Name 可能是动态混淆的。**策略**: 优先使用 ARIA 属性 (`aria-label`, `role="button"`) 或相对路径，避免硬编码随机类名。

2. **React 输入框**: 直接修改 `.innerHTML` 不会生效。**策略**: 使用剪贴板粘贴（`navigator.clipboard.writeText` + 模拟 Cmd+V）方式输入文本，更自然也更可靠。

3. **多 Tab 冲突**: 插件应强制只在一个 Gemini Tab 中工作。如果有多个，选取最近活动的一个。

4. **反检测**: 模拟人类操作行为，降低被检测为自动化的风险：
   * **输入方式**: 使用剪贴板粘贴代替直接 DOM 操作，符合人类复制粘贴的使用习惯。
   * **完整事件链**: 所有点击操作模拟完整的鼠标事件序列 (mouseover → mousedown → mouseup → click)。
   * **随机延时**: 操作间隔使用随机区间（如 300~800ms），避免固定间隔的机器特征。

---

## 8. 开发步骤

1. **Step 1**: 初始化 Golang Server，实现 WS 连接、心跳处理和基本的 GORM SQLite 结构。
2. **Step 2**: 初始化 Chrome Extension (Vite + TS)，配置 Manifest V3，实现 `background.ts` 连接 Server 并打印日志。
3. **Step 3**: 实现 Server 的 OpenAI 兼容 API 接口 (`POST /v1/chat/completions`)，支持流式与非流式，以及 WS 消息广播。
4. **Step 4 (最重要)**: 编写 `content.ts` 的 DOM 操作逻辑，实现精准的输入框定位、模拟输入和回复监听。
