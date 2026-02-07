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

- [x] 3.1 实现 `POST /v1/chat/completions` 端点，解析 OpenAI 格式请求体
- [x] 3.2 实现非流式响应：等待插件返回 DONE 后一次性返回 OpenAI 格式 JSON
- [x] 3.3 实现 SSE 流式响应：将插件的 PROCESSING/DONE 事件实时转为 SSE chunk（含差量计算）
- [x] 3.4 实现任务管理 (TaskManager)：请求 → UUID → 存 DB → WS 下发 → channel 等待回复 → 返回
- [x] 3.5 编写 API 接口测试 (16 tests all passed，含非流式、流式、无插件、无用户消息、任务分发)

## Step 4: 实现 Content Script DOM 操作

- [x] 4.1 实现悬浮窗 (overlay.ts, Shadow DOM 隔离样式，连接/任务状态，可拖拽，可最小化)
- [x] 4.2 实现输入框定位逻辑 (多种选择器 fallback)
- [x] 4.3 实现模拟输入 (`execCommand('insertText')` + input 事件触发)
- [x] 4.4 实现发送按钮定位与点击 (ARIA 属性 + fallback 按钮 + Enter 键)
- [x] 4.5 实现轮询监听回复：区分 PROCESSING 和 DONE 状态 (稳定性检测)
- [x] 4.6 实现回复内容提取 (model-response 容器 + 多种 fallback)
- [x] 4.7 实现多 Tab 管理 (已在 background.ts 中完成)
- [x] 4.8 构建并端到端手动测试：API → 插件操作 Gemini → 返回回复 ✅ 测试通过

## Bug 修复 & 功能优化

- [x] 修复 Quill 编辑器输入问题：多策略输入 (beforeinput → execCommand → paste → innerHTML)
- [x] 修复误点击麦克风按钮：限定 fallback 只查找 .send-button 类
- [x] 修复回复内容包含"显示思路"：优先从 .markdown 提取，排除 .model-thoughts 区域
- [x] 修复 WebSocket 频繁断连：readPump 中收到消息时刷新 ReadDeadline
- [x] 新对话支持：无 conversation_id 时模拟 Shift+Cmd+O 快捷键创建新对话
- [x] 模型选择：新对话时自动检查并确保选择 Pro 模型
- [x] 对话自动清理：回复完成后自动删除当前对话（菜单 → 删除 → 确认）
- [x] SSE 流式响应格式修复：ChatMessage 添加 omitempty 符合 OpenAI 规范

## 反检测优化

- [x] 5.1 输入方式改为剪贴板粘贴：写入系统剪贴板 → 模拟 Cmd+V 粘贴，更符合人类操作习惯
- [x] 5.2 完整鼠标事件链：所有点击操作增加 mouseover → mousedown → mouseup → click 事件序列
- [x] 5.3 随机化操作延时：所有 sleep 改为随机区间（如 300~800ms），消除固定间隔的机器特征
- [x] 5.4 构建并测试 ✅ 测试通过
