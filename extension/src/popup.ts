import { DEFAULT_CONFIG, ExtensionConfig } from "./types";

const wsUrlInput = document.getElementById("wsUrl") as HTMLInputElement;
const saveBtn = document.getElementById("saveBtn") as HTMLButtonElement;
const messageEl = document.getElementById("message") as HTMLDivElement;
const statusDot = document.getElementById("statusDot") as HTMLSpanElement;
const statusText = document.getElementById("statusText") as HTMLSpanElement;

// 加载已保存的配置
async function loadConfig(): Promise<void> {
  const result = await chrome.storage.local.get("config");
  const config: ExtensionConfig = result.config || DEFAULT_CONFIG;
  wsUrlInput.value = config.wsUrl;
}

// 保存配置
async function saveConfig(): Promise<void> {
  const config: ExtensionConfig = {
    wsUrl: wsUrlInput.value.trim() || DEFAULT_CONFIG.wsUrl,
  };

  await chrome.storage.local.set({ config });

  // 通知 background 重新连接
  chrome.runtime.sendMessage({ action: "reconnect" });

  showMessage("配置已保存");
}

// 查询连接状态
async function queryStatus(): Promise<void> {
  try {
    const response = await chrome.runtime.sendMessage({ action: "getStatus" });
    if (response?.connected) {
      statusDot.className = "status-dot connected";
      statusText.textContent = "已连接";
    } else {
      statusDot.className = "status-dot disconnected";
      statusText.textContent = "未连接";
    }
  } catch {
    statusDot.className = "status-dot disconnected";
    statusText.textContent = "未连接";
  }
}

function showMessage(text: string): void {
  messageEl.textContent = text;
  setTimeout(() => {
    messageEl.textContent = "";
  }, 2000);
}

// 初始化
loadConfig();
queryStatus();
saveBtn.addEventListener("click", saveConfig);
