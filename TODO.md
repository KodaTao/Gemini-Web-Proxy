# Gemini Web Proxy - 开发计划 (TODO.md)

## Step 1: 初始化 Golang Server 基础架构

- [x] 1.1 初始化 Go Module，安装依赖 (Gin, GORM, Gorilla WebSocket, SQLite, YAML)
- [x] 1.2 创建 config.yaml 和配置加载逻辑 (server/config/config.go)
- [x] 1.3 定义数据库模型 (Conversation, Message)，实现 GORM AutoMigrate
- [x] 1.4 实现 WebSocket `/ws` 端点，支持插件连接与消息收发
- [x] 1.5 实现心跳机制 (Server 每 30s 发送 PING，处理 PONG 响应)
- [x] 1.6 编写 Server 基础架构的单元测试 (11 tests passed)

## Step 2: 初始化 Chrome Extension 基础架构

- [x] 2.1 使用 Vite + TypeScript 初始化 Extension 项目，配置 Manifest V3
- [x] 2.2 实现共享类型定义 (types.ts)
- [x] 2.3 实现 Popup 配置页面 (popup.html/ts/css)，支持配置 WS 地址、显示连接状态
- [x] 2.4 实现 `background.ts`: WebSocket 连接、心跳响应 (PONG)、断线重连 (5s)、从 storage 读取配置、消息转发
- [x] 2.5 实现 `content.ts`: 消息监听框架 (chrome.runtime.onMessage)
- [x] 2.6 构建验证通过 (Vite build, dist 目录生成正确) ⏳ 等待用户在 Chrome 中手动加载插件测试

## Step 3: 实现 OpenAI 兼容 API

- [ ] 3.1 实现 `POST /v1/chat/completions` 端点，解析 OpenAI 格式请求体
- [ ] 3.2 实现非流式响应：等待插件返回 DONE 后一次性返回 OpenAI 格式 JSON
- [ ] 3.3 实现 SSE 流式响应：将插件的 PROCESSING/DONE 事件实时转为 SSE chunk
- [ ] 3.4 实现任务管理：请求 → 生成 UUID → 存 DB → 通过 WS 下发 → 等待回复 → 返回结果
- [ ] 3.5 编写 API 接口测试 (模拟 WS 插件回复)

## Step 4: 实现 Content Script DOM 操作

- [ ] 4.1 实现输入框定位逻辑 (优先 ARIA 属性，兼容多种 DOM 结构)
- [ ] 4.2 实现模拟输入 (`execCommand('insertText')` + input 事件触发)
- [ ] 4.3 实现发送按钮定位与点击
- [ ] 4.4 实现 MutationObserver 监听回复：区分 PROCESSING 和 DONE 状态
- [ ] 4.5 实现回复内容提取 (从 model-response 容器提取文本)
- [ ] 4.6 实现多 Tab 管理 (只在一个 Gemini Tab 工作，选最近活动的)
- [ ] 4.7 端到端手动测试：通过 API 发送消息 → 插件操作 Gemini → 返回回复
