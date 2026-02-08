/**
 * 悬浮窗模块 - 在 Gemini 页面上显示连接状态和任务状态
 * 使用 Shadow DOM 隔离样式
 */

export type ConnectionStatus = "connected" | "disconnected" | "connecting";
export type TaskStatus = "idle" | "processing" | "done" | "error";

const OVERLAY_STYLES = `
  :host {
    all: initial;
    position: fixed;
    bottom: 20px;
    right: 20px;
    z-index: 999999;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  }

  .overlay {
    background: #1e1e1e;
    color: #e0e0e0;
    border-radius: 8px;
    padding: 10px 14px;
    font-size: 12px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    cursor: move;
    user-select: none;
    min-width: 160px;
    border: 1px solid #333;
    transition: opacity 0.2s;
  }

  .overlay:hover {
    opacity: 1 !important;
  }

  .overlay.minimized {
    min-width: auto;
    padding: 6px 10px;
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
  }

  .title {
    font-weight: 600;
    font-size: 11px;
    color: #aaa;
    letter-spacing: 0.5px;
  }

  .header-btns {
    display: flex;
    gap: 4px;
  }

  .icon-btn {
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    font-size: 14px;
    padding: 0 2px;
    line-height: 1;
  }

  .icon-btn:hover {
    color: #fff;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 4px 0;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .dot.connected { background: #4caf50; }
  .dot.disconnected { background: #f44336; }
  .dot.connecting { background: #ff9800; animation: blink 1s infinite; }

  .dot.idle { background: #666; }
  .dot.processing { background: #2196f3; animation: blink 1s infinite; }
  .dot.done { background: #4caf50; }
  .dot.error { background: #f44336; }

  @keyframes blink {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.3; }
  }

  .label {
    color: #999;
    min-width: 32px;
  }

  .value {
    color: #e0e0e0;
  }

  /* 设置面板 */
  .settings-panel {
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid #444;
    display: none;
  }

  .settings-panel.open {
    display: block;
  }

  .settings-label {
    color: #999;
    font-size: 11px;
    margin-bottom: 4px;
  }

  .settings-input {
    width: 100%;
    box-sizing: border-box;
    background: #2a2a2a;
    border: 1px solid #555;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 12px;
    padding: 5px 8px;
    outline: none;
    font-family: monospace;
    cursor: text;
  }

  .settings-input:focus {
    border-color: #2196f3;
  }

  .settings-btn {
    margin-top: 6px;
    width: 100%;
    background: #2196f3;
    color: #fff;
    border: none;
    border-radius: 4px;
    padding: 5px 0;
    font-size: 12px;
    cursor: pointer;
    transition: background 0.2s;
  }

  .settings-btn:hover {
    background: #1976d2;
  }

  .settings-msg {
    color: #4caf50;
    font-size: 11px;
    margin-top: 4px;
    min-height: 14px;
  }
`;

export class Overlay {
  private host: HTMLElement;
  private shadow: ShadowRoot;
  private container: HTMLElement;
  private connDot!: HTMLElement;
  private connText!: HTMLElement;
  private taskDot!: HTMLElement;
  private taskText!: HTMLElement;
  private body!: HTMLElement;
  private minimizeBtn!: HTMLElement;
  private settingsBtn!: HTMLElement;
  private settingsPanel!: HTMLElement;
  private wsUrlInput!: HTMLInputElement;
  private settingsMsg!: HTMLElement;
  private isMinimized = false;
  private isSettingsOpen = false;

  // 拖拽状态
  private isDragging = false;
  private dragOffsetX = 0;
  private dragOffsetY = 0;

  constructor() {
    this.host = document.createElement("gemini-proxy-overlay");
    this.shadow = this.host.attachShadow({ mode: "closed" });

    // 注入样式
    const style = document.createElement("style");
    style.textContent = OVERLAY_STYLES;
    this.shadow.appendChild(style);

    // 构建 DOM
    this.container = document.createElement("div");
    this.container.className = "overlay";
    this.container.style.opacity = "0.85";
    this.container.innerHTML = `
      <div class="header">
        <span class="title">GEMINI PROXY</span>
        <div class="header-btns">
          <button class="icon-btn" data-id="settingsBtn" title="设置">⚙</button>
          <button class="icon-btn" data-id="minimizeBtn" title="最小化">−</button>
        </div>
      </div>
      <div class="body">
        <div class="row">
          <span class="dot disconnected" data-id="connDot"></span>
          <span class="label">连接</span>
          <span class="value" data-id="connText">未连接</span>
        </div>
        <div class="row">
          <span class="dot idle" data-id="taskDot"></span>
          <span class="label">任务</span>
          <span class="value" data-id="taskText">空闲</span>
        </div>
        <div class="settings-panel" data-id="settingsPanel">
          <div class="settings-label">WebSocket 地址</div>
          <input class="settings-input" data-id="wsUrlInput" type="text" placeholder="ws://localhost:6543/ws" />
          <button class="settings-btn" data-id="saveBtn">保存配置</button>
          <div class="settings-msg" data-id="settingsMsg"></div>
        </div>
      </div>
    `;

    this.shadow.appendChild(this.container);

    // 获取引用
    this.connDot = this.shadow.querySelector('[data-id="connDot"]')!;
    this.connText = this.shadow.querySelector('[data-id="connText"]')!;
    this.taskDot = this.shadow.querySelector('[data-id="taskDot"]')!;
    this.taskText = this.shadow.querySelector('[data-id="taskText"]')!;
    this.body = this.shadow.querySelector(".body")!;
    this.minimizeBtn = this.shadow.querySelector('[data-id="minimizeBtn"]')!;
    this.settingsBtn = this.shadow.querySelector('[data-id="settingsBtn"]')!;
    this.settingsPanel = this.shadow.querySelector('[data-id="settingsPanel"]')!;
    this.wsUrlInput = this.shadow.querySelector('[data-id="wsUrlInput"]')! as HTMLInputElement;
    this.settingsMsg = this.shadow.querySelector('[data-id="settingsMsg"]')!;
    const saveBtn = this.shadow.querySelector('[data-id="saveBtn"]')!;

    this.setupDrag();
    this.setupMinimize();
    this.setupSettings(saveBtn as HTMLElement);
  }

  mount(): void {
    document.body.appendChild(this.host);
  }

  setConnectionStatus(status: ConnectionStatus): void {
    this.connDot.className = `dot ${status}`;
    const labels: Record<ConnectionStatus, string> = {
      connected: "已连接",
      disconnected: "未连接",
      connecting: "连接中...",
    };
    this.connText.textContent = labels[status];
  }

  setTaskStatus(status: TaskStatus, detail?: string): void {
    this.taskDot.className = `dot ${status}`;
    const labels: Record<TaskStatus, string> = {
      idle: "空闲",
      processing: "处理中",
      done: "完成",
      error: "错误",
    };
    this.taskText.textContent = detail || labels[status];
  }

  private setupMinimize(): void {
    this.minimizeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      this.isMinimized = !this.isMinimized;
      if (this.isMinimized) {
        this.body.style.display = "none";
        this.container.classList.add("minimized");
        this.minimizeBtn.textContent = "+";
      } else {
        this.body.style.display = "";
        this.container.classList.remove("minimized");
        this.minimizeBtn.textContent = "−";
      }
    });
  }

  private setupSettings(saveBtn: HTMLElement): void {
    const DEFAULT_WS_URL = "ws://localhost:6543/ws";

    // 切换设置面板
    this.settingsBtn.addEventListener("click", async (e) => {
      e.stopPropagation();
      this.isSettingsOpen = !this.isSettingsOpen;
      if (this.isSettingsOpen) {
        this.settingsPanel.classList.add("open");
        // 加载当前配置
        try {
          const result = await chrome.storage.local.get("config");
          const config = result.config || { wsUrl: DEFAULT_WS_URL };
          this.wsUrlInput.value = config.wsUrl;
        } catch {
          this.wsUrlInput.value = DEFAULT_WS_URL;
        }
      } else {
        this.settingsPanel.classList.remove("open");
        this.settingsMsg.textContent = "";
      }
    });

    // 输入框阻止拖拽
    this.wsUrlInput.addEventListener("mousedown", (e) => {
      e.stopPropagation();
    });

    // 保存配置
    saveBtn.addEventListener("click", async (e) => {
      e.stopPropagation();
      const wsUrl = this.wsUrlInput.value.trim() || DEFAULT_WS_URL;
      const config = { wsUrl };
      await chrome.storage.local.set({ config });

      // 通知 background 重新连接
      chrome.runtime.sendMessage({ action: "reconnect" });

      this.settingsMsg.textContent = "✓ 已保存，正在重连...";
      setTimeout(() => {
        this.settingsMsg.textContent = "";
      }, 2000);
    });

    // 保存按钮也阻止拖拽
    saveBtn.addEventListener("mousedown", (e) => {
      e.stopPropagation();
    });
  }

  private setupDrag(): void {
    this.container.addEventListener("mousedown", (e: MouseEvent) => {
      if ((e.target as HTMLElement).classList.contains("icon-btn")) return;
      this.isDragging = true;
      const rect = this.host.getBoundingClientRect();
      this.dragOffsetX = e.clientX - rect.left;
      this.dragOffsetY = e.clientY - rect.top;
      e.preventDefault();
    });

    document.addEventListener("mousemove", (e: MouseEvent) => {
      if (!this.isDragging) return;
      const x = e.clientX - this.dragOffsetX;
      const y = e.clientY - this.dragOffsetY;
      this.host.style.position = "fixed";
      this.host.style.left = `${x}px`;
      this.host.style.top = `${y}px`;
      this.host.style.right = "auto";
      this.host.style.bottom = "auto";
    });

    document.addEventListener("mouseup", () => {
      this.isDragging = false;
    });
  }
}
