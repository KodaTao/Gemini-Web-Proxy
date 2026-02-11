import type {InternalMessage, WSMessage} from "./types";
import {Overlay} from "./overlay";
import * as cheerio from 'cheerio';
import TurndownService from 'turndown';
import {gfm} from 'turndown-plugin-gfm';

console.log("[Content] Gemini Web Proxy content script loaded");

// ========== 悬浮窗 ==========
const overlay = new Overlay();
overlay.mount();

// 定期查询连接状态
async function pollConnectionStatus(): Promise<void> {
  try {
    const response = await chrome.runtime.sendMessage({ action: "getStatus" });
    overlay.setConnectionStatus(response?.connected ? "connected" : "disconnected");
  } catch {
    overlay.setConnectionStatus("disconnected");
  }
}
pollConnectionStatus();
setInterval(pollConnectionStatus, 3000);

// ========== DOM 操作工具函数 ==========

/**
 * 定位输入框 - 多种选择器 fallback
 */
function findInputElement(): HTMLElement | null {
  const selectors = [
    // 新版 Gemini UI (2025+): rich-textarea 组件 + new-input-ui
    'rich-textarea .ql-editor.new-input-ui[contenteditable="true"]',
    'rich-textarea .ql-editor[contenteditable="true"][role="textbox"]',
    'div[contenteditable="true"][aria-label="在此处输入提示"]',
    'div[contenteditable="true"][aria-label*="Enter a prompt"]',
    // 旧版 Gemini Quill 编辑器
    'div.ql-editor[contenteditable="true"][role="textbox"]',
    'div.ql-editor.textarea[contenteditable="true"]',
    '.ql-editor[contenteditable="true"]',
    // 通过 aria-label 定位（中文/英文）
    'div[contenteditable="true"][aria-label*="提示"]',
    'div[contenteditable="true"][aria-label*="prompt"]',
    // 通过容器定位
    '.text-input-field_textarea-inner div[contenteditable="true"]',
    '.text-input-field_textarea div[contenteditable="true"]',
    // 宽泛 fallback
    'div[contenteditable="true"][role="textbox"]',
  ];

  for (const selector of selectors) {
    const el = document.querySelector<HTMLElement>(selector);
    if (el) {
      console.log("[Content] found input with selector:", selector);
      return el;
    }
  }
  return null;
}

/**
 * 模拟输入文本到输入框
 * 使用剪贴板粘贴方式，最接近人类操作习惯
 */
async function simulateInput(inputEl: HTMLElement, text: string): Promise<void> {
  // 聚焦输入框（模拟点击聚焦）
  simulateClick(inputEl);
  await randomDelay(100, 300);
  inputEl.focus();

  // 清空现有内容
  const selection = window.getSelection();
  if (selection) {
    selection.selectAllChildren(inputEl);
    selection.deleteFromDocument();
  }

  // 策略1（首选）：写入系统剪贴板 + 模拟 Cmd+V 粘贴
  try {
    await navigator.clipboard.writeText(text);
    // 模拟 Cmd+V / Ctrl+V 快捷键
    const isMac = navigator.platform.toUpperCase().includes("MAC");
    inputEl.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "v",
        code: "KeyV",
        keyCode: 86,
        [isMac ? "metaKey" : "ctrlKey"]: true,
        bubbles: true,
        cancelable: true,
      })
    );
    // 同时触发 paste 事件（浏览器安全策略可能需要）
    const dataTransfer = new DataTransfer();
    dataTransfer.setData("text/plain", text);
    inputEl.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData: dataTransfer,
        bubbles: true,
        cancelable: true,
      })
    );
    await randomDelay(100, 200);
    // 检查输入框是否有内容（不用全量对比，因为 XML 标签会被 HTML 解析后 textContent 不同）
    const pastedContent = inputEl.textContent?.trim() || "";
    if (pastedContent.length > 0) {
      console.log("[Content] input via clipboard paste succeeded, length:", pastedContent.length);
      return;
    }
  } catch (e) {
    console.log("[Content] clipboard paste failed:", e);
  }

  // 策略2：使用 execCommand（传统方式）
  inputEl.focus();
  const execResult = document.execCommand("insertText", false, text);
  if (execResult && (inputEl.textContent?.trim().length || 0) > 0) {
    console.log("[Content] input via execCommand succeeded");
    inputEl.dispatchEvent(new Event("input", { bubbles: true }));
    return;
  }

  // 策略3：直接操作 DOM（最后手段，需转义 HTML 特殊字符）
  console.log("[Content] all input methods failed, using direct DOM manipulation");
  const escaped = text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\n/g, "<br>");
  inputEl.innerHTML = `<p>${escaped}</p>`;
  inputEl.dispatchEvent(new Event("input", { bubbles: true }));
  inputEl.dispatchEvent(new Event("change", { bubbles: true }));
}

/**
 * 定位并点击发送按钮
 */
function findAndClickSendButton(): boolean {
  const selectors = [
    // Gemini 实际使用的发送按钮
    'button.send-button.submit[aria-label="发送"]',
    'button.send-button.submit',
    'button.send-button[aria-label="发送"]',
    'button[aria-label="发送"]:not(.stop)',
    // 英文版
    'button.send-button[aria-label="Send"]',
    'button[aria-label="Send message"]',
    // 通过容器定位
    '.send-button-container button.send-button:not(.stop)',
  ];

  for (const selector of selectors) {
    const btn = document.querySelector<HTMLButtonElement>(selector);
    if (btn && btn.getAttribute("aria-disabled") !== "true") {
      console.log("[Content] clicking send button:", selector);
      simulateClick(btn);
      return true;
    }
  }

  // Fallback：在 send-button-container 中查找，只找 .send-button 类的按钮
  // 避免误点击麦克风等其他按钮
  const container = document.querySelector(".send-button-container, .input-buttons-wrapper-bottom");
  if (container) {
    const btn = container.querySelector<HTMLButtonElement>("button.send-button:not(.stop)");
    if (btn && btn.getAttribute("aria-disabled") !== "true") {
      console.log("[Content] clicking fallback send button");
      simulateClick(btn);
      return true;
    }
  }

  return false;
}

/**
 * 获取当前对话 ID（从 URL 中提取）
 */
function getConversationId(): string {
  const match = window.location.pathname.match(/\/app\/([^/?]+)/);
  return match ? match[1] : "";
}

/**
 * 从元素中提取文本，排除思考过程区域
 * Gemini 的"显示思路"按钮及思考内容位于 .model-thoughts / .thoughts-container 中
 * 实际回复内容在 .markdown 中
 */
function extractTextWithoutThinking(el: HTMLElement): string {
  // 克隆节点以避免修改原 DOM
  const clone = el.cloneNode(true) as HTMLElement;

  // 移除思考过程相关元素（基于实际 Gemini DOM 结构）
  const thinkingSelectors = [
    ".model-thoughts",
    ".thoughts-container",
    ".thoughts-content",
    ".thoughts-wrapper",
    ".thoughts-header",
    ".thoughts-header-button",
    '[class*="thoughts"]',
  ];

  for (const sel of thinkingSelectors) {
    clone.querySelectorAll(sel).forEach((node) => node.remove());
  }

  // 兜底：正则去除"显示思路"/"隐藏思路"文本
  let text = clone.innerText.trim();
  text = text.replace(/^(显示思路|隐藏思路|Show thinking|Hide thinking)\s*/i, "");
  text = text.replace(/\n(显示思路|隐藏思路|Show thinking|Hide thinking)\s*/gi, "\n");

  return text.trim();
}

/**
 * 获取最后一个 model 回复的文本内容
 */
function getLastModelResponse(): Array<string> {
  // 获取所有 model 回复，取最后一个
  const modelSelectors = [
    'model-response',
    '[data-message-author-role="model"]',
    '[data-message-author-role="assistant"]',
  ];

  for (const selector of modelSelectors) {
    const allResponses = document.querySelectorAll<HTMLElement>(selector);
    if (allResponses.length > 0) {
      const last = allResponses[allResponses.length - 1];
      const h = last.innerHTML;
      // 优先从 .markdown 获取纯回复文本（不含思考过程）
      // Gemini DOM: model-response > response-container > model-response-text > message-content > .markdown
      const markdownEl = last.querySelector<HTMLElement>(".markdown");
      if (markdownEl) {
        const text = markdownEl.innerText.trim();
        if (text&&h) return [text,h];
      }
      // 备选：从 message-content 获取
      const msgContent = last.querySelector<HTMLElement>("message-content");
      if (msgContent) {
        const text = msgContent.innerText.trim();
        if (text&&h) return [text, h];
      }
      // 最后手段：从整个元素提取，排除思考区域
      const text = extractTextWithoutThinking(last);
      if (text&&h) return [text, h];
    }
  }

  return ["", ""];
}

/**
 * 检测是否正在生成中
 */
function isGenerating(): boolean {
  // 检测 Gemini 的停止按钮（正在生成时显示 .stop 类）
  const stopBtn = document.querySelector('button.send-button.stop:not([aria-disabled="true"])');
  if (stopBtn) return true;

  // 备用：检测 aria-label
  const stopSelectors = [
    'button[aria-label="Stop generating"]',
    'button[aria-label="停止生成"]',
    'button[aria-label="Stop"]',
  ];
  for (const selector of stopSelectors) {
    if (document.querySelector(selector)) return true;
  }

  return false;
}

// ========== 新对话 & 模型选择 ==========

/**
 * 判断当前是否在已有对话中（URL 包含 /app/xxxxx）
 */
function isInExistingConversation(): boolean {
  return /\/app\/[a-zA-Z0-9]+/.test(window.location.pathname);
}

/**
 * 创建新对话：模拟 Shift+Command+O 快捷键
 */
async function startNewConversation(): Promise<boolean> {
  // 如果当前已在首页（/app 且不是 /app/xxxx），无需新建
  if (!isInExistingConversation()) {
    console.log("[Content] already on new conversation page");
    return true;
  }

  console.log("[Content] creating new conversation via Shift+Cmd+O");

  // 模拟 Shift+Command+O 快捷键
  const keyEvent = new KeyboardEvent("keydown", {
    key: "O",
    code: "KeyO",
    keyCode: 79,
    shiftKey: true,
    metaKey: true,
    bubbles: true,
    cancelable: true,
  });
  document.dispatchEvent(keyEvent);

  // 等待页面导航完成
  await waitForNewPage();
  return true;
}

/**
 * 等待页面变为新对话状态
 */
async function waitForNewPage(): Promise<void> {
  for (let i = 0; i < 20; i++) {
    await randomDelay(400, 700);
    // 检查是否已加载完成：输入框存在 && 不在旧对话中
    const input = findInputElement();
    if (input && !isInExistingConversation()) {
      console.log("[Content] new conversation page ready");
      return;
    }
  }
  console.log("[Content] timeout waiting for new page");
}

/**
 * 获取当前选择的模型名称
 */
function getCurrentModelName(): string {
  // 输入框区域的模型选择器按钮
  const labelContainer = document.querySelector(
    '[data-test-id="bard-mode-menu-button"] [data-test-id="logo-pill-label-container"] span,' +
    '[data-test-id="bard-mode-menu-button"] .input-area-switch-label span'
  );
  if (labelContainer) {
    return labelContainer.textContent?.trim() || "";
  }
  return "";
}

/**
 * 确保选择了 Pro 模型
 * 如果当前已是 Pro，直接返回；否则打开选择器切换
 */
async function ensureProModel(): Promise<void> {
  const currentModel = getCurrentModelName();
  console.log("[Content] current model:", currentModel);

  if (currentModel.toLowerCase().includes("pro")) {
    console.log("[Content] already using Pro model");
    return;
  }

  // 点击模型选择器按钮，打开下拉菜单
  const menuButton = document.querySelector<HTMLElement>(
    '[data-test-id="bard-mode-menu-button"] button, [data-test-id="bard-mode-menu-button"]'
  );
  if (!menuButton) {
    console.log("[Content] model picker button not found, skipping");
    return;
  }

  simulateClick(menuButton);
  await randomDelay(400, 700);

  // 在下拉菜单中查找包含 "Pro" 的选项
  const menuItems = document.querySelectorAll<HTMLElement>(
    '.mat-mdc-menu-panel button, .mat-mdc-menu-panel [role="menuitem"], .cdk-overlay-pane button'
  );
  for (const item of menuItems) {
    const text = item.textContent?.trim() || "";
    if (text.includes("Pro") && !text.includes("Ultra")) {
      console.log("[Content] selecting Pro model:", text);
      simulateClick(item);
      await randomDelay(400, 700);
      return;
    }
  }

  console.log("[Content] Pro option not found in menu, closing menu");
  // 关闭菜单（按 Escape）
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
}

// ========== 对话管理 ==========

/**
 * 删除当前对话：打开菜单 → 点击删除 → 确认
 */
async function deleteCurrentConversation(): Promise<void> {
  console.log("[Content] deleting current conversation");

  // 1. 点击对话操作菜单按钮
  const menuBtn = document.querySelector<HTMLElement>(
    '[data-test-id="actions-menu-button"]'
  );
  if (!menuBtn) {
    console.log("[Content] actions menu button not found, skipping delete");
    return;
  }
  menuBtn.click();
  await randomDelay(400, 700);

  // 2. 点击删除按钮
  const deleteBtn = document.querySelector<HTMLElement>(
    '[data-test-id="delete-button"]'
  );
  if (!deleteBtn) {
    console.log("[Content] delete button not found, closing menu");
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    return;
  }
  deleteBtn.click();
  await randomDelay(400, 700);

  // 3. 等待确认弹窗出现并点击确认（Angular Material 按钮需要原生 .click()）
  for (let i = 0; i < 6; i++) {
    const confirmBtn = document.querySelector<HTMLElement>(
      'button[data-test-id="confirm-button"]'
    );
    if (confirmBtn) {
      console.log("[Content] clicking confirm button");
      confirmBtn.click();
      await randomDelay(300, 600);
      console.log("[Content] conversation deleted");
      return;
    }
    await randomDelay(200, 400);
  }

  console.log("[Content] confirm button not found after retries");
}

/**
 * 点击最后一个 model 回复的复制按钮，从剪贴板获取 Markdown 格式内容
 */
/**
 * 检测"已复制到剪贴板"snackbar 弹窗是否出现
 */
function detectCopySnackbar(): boolean {
  const snackbar = document.querySelector("simple-snack-bar .mat-mdc-snack-bar-label");
  if (snackbar) {
    const text = snackbar.textContent?.trim() || "";
    if (text.includes("已复制到剪贴板") || text.toLowerCase().includes("copied to clipboard")) {
      return true;
    }
  }
  return false;
}

/**
 * 等待复制成功的 snackbar 弹窗出现
 * @returns true 如果检测到弹窗，false 超时
 */
async function waitForCopySnackbar(timeoutMs = 2000): Promise<boolean> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (detectCopySnackbar()) {
      return true;
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  return false;
}

/**
 * 从剪贴板读取内容（支持 Clipboard API 和 Safari fallback）
 */
async function readClipboardText(): Promise<string | null> {
  // 方式1：使用 Clipboard API 读取（Chrome 推荐）
  try {
    const text = await navigator.clipboard.readText();
    if (text && text.trim().length > 0) {
      return text.trim();
    }
  } catch (e) {
    console.log("[Content] Clipboard API readText failed (may be Safari restriction):", e);
  }

  // 方式2：Safari fallback — 通过隐藏 textarea + execCommand('paste') 读取剪贴板
  try {
    const textarea = document.createElement("textarea");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    textarea.style.top = "-9999px";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    const pasteResult = document.execCommand("paste");
    const pastedText = textarea.value;
    document.body.removeChild(textarea);
    if (pasteResult && pastedText && pastedText.trim().length > 0) {
      return pastedText.trim();
    }
  } catch (e) {
    console.log("[Content] execCommand paste fallback also failed:", e);
  }

  return null;
}

async function clickCopyAndGetMarkdown(htmlContent: string): Promise<string | null> {
    try {
        // 1. 加载 HTML
        const $ = cheerio.load(htmlContent);

        // 2. 定位核心内容区域
        const targetContainer = $('.markdown-main-panel');
        if (targetContainer.length === 0) {
            return null;
        }

        // --- 3. DOM 清洗与预处理 ---

        // 3.1 移除不需要的 UI 元素
        targetContainer.find('.table-footer').remove();
        targetContainer.find('button').remove();
        targetContainer.find('mat-icon').remove();
        targetContainer.find('.message-actions').remove();
        targetContainer.find('model-thoughts').remove();

        // 3.2 【关键修改】预处理表格单元格，将块级元素改为行内元素 + <br>
        // 这一步是为了防止 Turndown 自动把 <p> 转成 \n\n
        targetContainer.find('td, th').each((_, cell) => {
            const $cell = $(cell);

            // 找到单元格内的所有段落 <p> 和 <div>
            // 比如 <td><p>Line 1</p><p>Line 2</p></td>
            $cell.find('p, div, li').each((_, block) => {
                const $block = $(block);
                // 将其替换为：内容 + <br>
                // 结果变成：<td>Line 1<br>Line 2<br></td>
                $block.replaceWith($block.html() + '<br>');
            });

            // (可选) 如果不需要每个单元格最后都有个 <br>，可以清理一下结尾
            // 但通常保留也无伤大雅，Markdown渲染器会忽略末尾的 <br>
        });

        // 4. 获取清洗后的 HTML
        const cleanHtml = targetContainer.html() || '';

        // 5. 配置 Turndown
        const turndownService = new TurndownService({
            headingStyle: 'atx',
            hr: '---',
            bulletListMarker: '-',
            codeBlockStyle: 'fenced',
        });

        // 6. 启用 GFM 插件 (必须用于支持表格)
        turndownService.use(gfm);

        // 7. 【关键修改】添加自定义规则：表格内的 <br> 禁止转为 \n
        turndownService.addRule('keep-br-in-tables', {
            filter: 'br',
            replacement: function (_content, node) {
                // 向上查找，判断当前 <br> 是否在表格内
                let parent = node.parentNode;
                let isInsideTable = false;

                while (parent) {
                    // 如果找到了 TD 或 TH，说明在表格里
                    if (parent.nodeName === 'TD' || parent.nodeName === 'TH') {
                        isInsideTable = true;
                        break;
                    }
                    // 如果找到了 BODY 或 TABLE 还没找到单元格，就停止
                    if (parent.nodeName === 'TABLE' || parent.nodeName === 'BODY') {
                        break;
                    }
                    parent = parent.parentNode;
                }

                if (isInsideTable) {
                    // 如果在表格里，输出 HTML 的 <br> 标签字符串
                    // 这样 Markdown 解析器会认为它是单元格内的换行，而不是表格行的结束
                    return '<br>';
                }

                // 如果不在表格里，返回标准的 Markdown 换行符
                return '\n';
            }
        });

        // 8. 执行转换
        const markdown = turndownService.turndown(cleanHtml);

        // 9. (兜底) 最后再做一次正则替换，防止漏网之鱼
        // 有时候 GFM 插件内部处理可能会产生漏网的 \n，我们在字符串层面做最后一次清洗
        // 匹配表格行（以 | 开头和结尾的行），如果中间有 \n 则替换为空格或 <br>，
        // 但正则处理 Markdown 表格非常复杂，通常上面的 Step 7 已经足够解决问题。

        return markdown;

    } catch (error) {
        console.error('转换 Markdown 失败:', error);
        return null;
    }
}
// ========== 核心消息处理 ==========

/**
 * 处理来自 Server 的发送消息指令
 */
async function handleSendMessage(wsMsg: WSMessage): Promise<void> {
  const taskId = wsMsg.id || "";
  const payload = wsMsg.payload as { prompt: string; conversation_id: string } | undefined;

  if (!payload?.prompt) {
    sendError(taskId, "no prompt in payload");
    return;
  }

  // 上报忙碌状态
  sendStatus("busy");
  overlay.setTaskStatus("processing", "准备中...");
  console.log("[Content] sending prompt:", payload.prompt.substring(0, 50) + "...");

  // 0. 如果没有 conversation_id，先创建新对话并选择 Pro 模型
  if (!payload.conversation_id) {
    overlay.setTaskStatus("processing", "创建新对话...");
    await startNewConversation();
    await ensureProModel();
  }

  overlay.setTaskStatus("processing", "发送中...");

  // 1. 定位输入框
  const inputEl = findInputElement();
  if (!inputEl) {
    overlay.setTaskStatus("error", "找不到输入框");
    sendError(taskId, "cannot find input element");
    sendStatus("idle");
    return;
  }

  // 2. 模拟输入
  simulateInput(inputEl, payload.prompt);

  // 3. 等待发送按钮变为可用并点击（输入后按钮可能需要一些时间才会启用）
  let sent = false;
  for (let retry = 0; retry < 6; retry++) {
    await randomDelay(400, 800);
    // 检查输入是否成功（输入框中是否有文本）
    const hasText = inputEl.textContent && inputEl.textContent.trim().length > 0;
    console.log(`[Content] retry ${retry}: hasText=${hasText}`);

    if (findAndClickSendButton()) {
      sent = true;
      break;
    }
  }

  if (!sent) {
    // 尝试 Enter 键发送
    console.log("[Content] send button not available after retries, trying Enter key");
    inputEl.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "Enter",
        code: "Enter",
        keyCode: 13,
        bubbles: true,
      })
    );
  }

  overlay.setTaskStatus("processing", "等待回复...");

  // 4. 等待并监听回复
  await randomDelay(1500, 2500); // 等待 Gemini 开始生成
  watchForReply(taskId);
}

/**
 * 使用轮询方式监听回复（比 MutationObserver 更稳定）
 */
function watchForReply(taskId: string): void {
  let lastText = "";
  let stableCount = 0;
  const STABLE_THRESHOLD = 3; // 文本连续 3 次不变 && 非生成中 => DONE
  const POLL_INTERVAL = 1000; // 每秒检查
  const MAX_WAIT = 120000; // 最长等待 2 分钟
  const startTime = Date.now();

  const pollTimer = setInterval(() => {
    const elapsed = Date.now() - startTime;
    if (elapsed > MAX_WAIT) {
      clearInterval(pollTimer);
      overlay.setTaskStatus("error", "超时");
      sendError(taskId, "response timeout");
      sendStatus("idle");
      return;
    }

    const currentTextAndHtml = getLastModelResponse();
    const currentText = currentTextAndHtml[0];
    const currentHtml = currentTextAndHtml[1];
    const generating = isGenerating();

    if (currentText && currentText !== lastText) {
      // 文本有变化，发送 PROCESSING
      lastText = currentText;
      stableCount = 0;
      sendReply(taskId, currentText, "PROCESSING");
      overlay.setTaskStatus("processing", `生成中 (${Math.floor(elapsed / 1000)}s)`);
    } else if (currentText && !generating) {
      // 文本没变且不在生成中
      stableCount++;
      if (stableCount >= STABLE_THRESHOLD) {
        // 稳定了，先点击复制按钮获取 Markdown 内容，再发送 DONE
        clearInterval(pollTimer);
        overlay.setTaskStatus("processing", "复制内容...");
        clickCopyAndGetMarkdown(currentHtml).then(async (markdown) => {
          const finalText = markdown || currentText;
          // 先删除对话，再发送 DONE
          overlay.setTaskStatus("processing", "删除对话...");
          await deleteCurrentConversation();
          sendReply(taskId, finalText, "DONE");
          overlay.setTaskStatus("idle");
          sendStatus("idle");
        }).catch(async () => {
          // 即使复制失败也用 DOM 提取的文本兜底
          overlay.setTaskStatus("processing", "删除对话...");
          await deleteCurrentConversation().catch(() => {});
          sendReply(taskId, currentText, "DONE");
          overlay.setTaskStatus("idle");
          sendStatus("idle");
        });
      }
    } else {
      stableCount = 0;
    }
  }, POLL_INTERVAL);
}

// ========== 消息发送工具 ==========

function sendReply(taskId: string, text: string, status: "PROCESSING" | "DONE"): void {
  const conversationId = getConversationId();
  chrome.runtime.sendMessage({
    action: "wsReply",
    data: {
      reply_to: taskId,
      type: "EVENT_REPLY",
      payload: { text, status, conversation_id: conversationId },
    },
  });
}

function sendError(taskId: string, error: string): void {
  chrome.runtime.sendMessage({
    action: "wsReply",
    data: {
      reply_to: taskId,
      type: "EVENT_ERROR",
      payload: { error },
    },
  });
}

function sendStatus(status: "idle" | "busy"): void {
  console.log(`[Content] reporting status: ${status}`);
  chrome.runtime.sendMessage({
    action: "wsReply",
    data: {
      type: "EVENT_STATUS",
      payload: { status },
    },
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * 随机延时，模拟人类操作间隔
 */
function randomDelay(min: number, max: number): Promise<void> {
  const ms = Math.floor(Math.random() * (max - min + 1)) + min;
  return sleep(ms);
}

/**
 * 模拟完整的鼠标点击事件链：mouseover → mousedown → mouseup → click
 * 比直接调用 .click() 更接近真实用户行为
 */
function simulateClick(el: HTMLElement): void {
  const rect = el.getBoundingClientRect();
  const x = rect.left + rect.width / 2;
  const y = rect.top + rect.height / 2;

  const commonInit: MouseEventInit = {
    bubbles: true,
    cancelable: true,
    view: window,
    clientX: x,
    clientY: y,
  };

  el.dispatchEvent(new MouseEvent("mouseover", commonInit));
  el.dispatchEvent(new MouseEvent("mousedown", { ...commonInit, button: 0 }));
  el.dispatchEvent(new MouseEvent("mouseup", { ...commonInit, button: 0 }));
  el.dispatchEvent(new MouseEvent("click", { ...commonInit, button: 0 }));
}

// ========== 消息监听 ==========

chrome.runtime.onMessage.addListener(
  (message: InternalMessage, _sender, sendResponse) => {
    if (message.action === "sendMessage") {
      const wsMsg = message.data as WSMessage;
      console.log("[Content] received command:", wsMsg.type, wsMsg.id);
      handleSendMessage(wsMsg);
      sendResponse({ received: true });
    }
    return true;
  }
);
