// WebSocket 消息类型
export interface WSMessage {
  id?: string;
  reply_to?: string;
  type: string;
  payload?: Record<string, unknown>;
}

// 服务端 -> 插件 指令
export interface CmdSendMessage extends WSMessage {
  type: "CMD_SEND_MESSAGE";
  payload: {
    prompt: string;
    conversation_id: string;
  };
}

// 插件 -> 服务端 事件
export interface EventReply extends WSMessage {
  type: "EVENT_REPLY";
  reply_to: string;
  payload: {
    text: string;
    status: "PROCESSING" | "DONE";
    conversation_id: string;
  };
}

export interface EventError extends WSMessage {
  type: "EVENT_ERROR";
  reply_to: string;
  payload: {
    error: string;
  };
}

// Chrome 内部消息（Background <-> Content Script）
export interface InternalMessage {
  action: string;
  data?: unknown;
}

// 插件配置
export interface ExtensionConfig {
  wsUrl: string;
}

export const DEFAULT_CONFIG: ExtensionConfig = {
  wsUrl: "ws://localhost:8080/ws",
};
