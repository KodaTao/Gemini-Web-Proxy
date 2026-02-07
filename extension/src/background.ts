import { WSMessage, DEFAULT_CONFIG, ExtensionConfig } from "./types";

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let connected = false;

const RECONNECT_INTERVAL = 5000;

// 从 storage 读取配置
async function getConfig(): Promise<ExtensionConfig> {
  const result = await chrome.storage.local.get("config");
  return result.config || DEFAULT_CONFIG;
}

// 建立 WebSocket 连接
async function connect(): Promise<void> {
  // 清理旧连接
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  const config = await getConfig();
  console.log(`[BG] connecting to ${config.wsUrl}`);

  try {
    ws = new WebSocket(config.wsUrl);
  } catch (err) {
    console.error("[BG] WebSocket creation failed:", err);
    scheduleReconnect();
    return;
  }

  ws.onopen = () => {
    connected = true;
    console.log("[BG] WebSocket connected");
  };

  ws.onmessage = (event: MessageEvent) => {
    try {
      const msg: WSMessage = JSON.parse(event.data);
      handleServerMessage(msg);
    } catch (err) {
      console.error("[BG] invalid message:", err);
    }
  };

  ws.onclose = () => {
    connected = false;
    ws = null;
    console.log("[BG] WebSocket disconnected");
    scheduleReconnect();
  };

  ws.onerror = (err) => {
    console.error("[BG] WebSocket error:", err);
  };
}

// 定时重连
function scheduleReconnect(): void {
  if (reconnectTimer) return;
  console.log(`[BG] will reconnect in ${RECONNECT_INTERVAL / 1000}s`);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect();
  }, RECONNECT_INTERVAL);
}

// 发送消息到 Server
function sendToServer(msg: WSMessage): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg));
  } else {
    console.error("[BG] cannot send, WebSocket not connected");
  }
}

// 处理服务端消息
function handleServerMessage(msg: WSMessage): void {
  console.log(`[BG] received: type=${msg.type}`);

  switch (msg.type) {
    case "PING":
      sendToServer({ type: "PONG" });
      break;

    case "CMD_SEND_MESSAGE":
      forwardToContentScript(msg);
      break;

    default:
      console.log(`[BG] unknown message type: ${msg.type}`);
  }
}

// 转发指令到 Content Script
async function forwardToContentScript(msg: WSMessage): Promise<void> {
  try {
    // 查找 Gemini tab
    const tabs = await chrome.tabs.query({ url: "https://gemini.google.com/*" });

    let tab: chrome.tabs.Tab | undefined;

    if (tabs.length === 0) {
      // 没有 Gemini tab，创建一个
      console.log("[BG] no Gemini tab found, creating one");
      tab = await chrome.tabs.create({ url: "https://gemini.google.com/app", active: false });

      // 等待页面加载完成
      await new Promise<void>((resolve) => {
        const listener = (tabId: number, info: chrome.tabs.TabChangeInfo) => {
          if (tabId === tab!.id && info.status === "complete") {
            chrome.tabs.onUpdated.removeListener(listener);
            resolve();
          }
        };
        chrome.tabs.onUpdated.addListener(listener);
      });

      // 额外等待，确保 content script 注入完成
      await new Promise((resolve) => setTimeout(resolve, 2000));
    } else {
      // 选取最近活动的 tab
      tab = tabs.sort((a, b) => (b.lastAccessed || 0) - (a.lastAccessed || 0))[0];
    }

    if (!tab?.id) {
      sendToServer({
        type: "EVENT_ERROR",
        reply_to: msg.id,
        payload: { error: "cannot find or create Gemini tab" },
      });
      return;
    }

    // 转发给 content script
    const response = await chrome.tabs.sendMessage(tab.id, {
      action: "sendMessage",
      data: msg,
    });

    console.log("[BG] content script response:", response);
  } catch (err) {
    console.error("[BG] forward to content script failed:", err);
    sendToServer({
      type: "EVENT_ERROR",
      reply_to: msg.id,
      payload: { error: `forward failed: ${err}` },
    });
  }
}

// 监听来自 content script 和 popup 的消息
chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (message.action === "reconnect") {
    connect();
    sendResponse({ ok: true });
  } else if (message.action === "getStatus") {
    sendResponse({ connected });
  } else if (message.action === "wsReply") {
    // Content Script 回传的消息，转发给 Server
    sendToServer(message.data as WSMessage);
    sendResponse({ ok: true });
  }
  return true; // 保持 sendResponse 可用
});

// 启动连接
connect();
