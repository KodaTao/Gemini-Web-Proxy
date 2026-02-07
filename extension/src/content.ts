import type { InternalMessage, WSMessage } from "./types";

console.log("[Content] Gemini Web Proxy content script loaded");

// 监听来自 Background 的消息
chrome.runtime.onMessage.addListener(
  (message: InternalMessage, _sender, sendResponse) => {
    if (message.action === "sendMessage") {
      const wsMsg = message.data as WSMessage;
      console.log("[Content] received command:", wsMsg.type, wsMsg.id);

      // TODO: Step 4 实现 DOM 操作
      // 1. 定位输入框
      // 2. 模拟输入 prompt
      // 3. 点击发送按钮
      // 4. 监听回复 (MutationObserver)
      // 5. 回传结果给 background

      sendResponse({ received: true });
    }
    return true;
  }
);
