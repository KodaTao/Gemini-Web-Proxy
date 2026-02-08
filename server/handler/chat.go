package handler

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/KodaTao/Gemini-Web-Proxy/server/model"
)

// XML 序列化结构：将 OpenAI messages 转为 XML 格式发送给 Gemini
type CData struct {
	Value string `xml:",cdata"`
}

type PromptXmlMessage struct {
	Role    string `xml:"role,attr"`
	Content CData  `xml:",innerxml"`
}

type PromptXml struct {
	XMLName  xml.Name            `xml:"chat_history"`
	Messages []*PromptXmlMessage `xml:"message"`
}

// messagesToXML 将 OpenAI 格式的 messages 序列化为 XML 字符串
func messagesToXML(messages []ChatMessage) (string, error) {
	promptXml := &PromptXml{}
	for _, msg := range messages {
		promptXml.Messages = append(promptXml.Messages, &PromptXmlMessage{
			Role: msg.Role,
			Content: CData{
				Value: "\n" + msg.Content + "\n",
			},
		})
	}
	data, err := xml.MarshalIndent(promptXml, "", "    ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

const requestTimeout = 120 * time.Second

// OpenAI 兼容请求/响应结构

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatHandler 处理 /v1/chat/completions 请求
type ChatHandler struct {
	Hub         *Hub
	TaskManager *TaskManager
	DB          *gorm.DB
	semaphore   chan struct{} // 并发限制：同一时间只允许一个请求
	apiKey      string        // API Key，为空则不验证
}

// NewChatHandler 创建 ChatHandler 实例
func NewChatHandler(hub *Hub, tm *TaskManager, db *gorm.DB, apiKey string) *ChatHandler {
	return &ChatHandler{
		Hub:         hub,
		TaskManager: tm,
		DB:          db,
		semaphore:   make(chan struct{}, 1),
		apiKey:      apiKey,
	}
}

func (h *ChatHandler) Handle(c *gin.Context) {
	// 并发控制：server 端信号量 + 插件端状态双重检查
	select {
	case h.semaphore <- struct{}{}:
		defer func() { <-h.semaphore }()
	default:
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"message": "server is already processing a request, please try again later",
				"type":    "rate_limit_error",
			},
		})
		return
	}

	// API Key 验证
	if h.apiKey != "" {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "missing Authorization header",
					"type":    "authentication_error",
				},
			})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth || token != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "invalid API key",
					"type":    "authentication_error",
				},
			})
			return
		}
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	// 检查是否包含 user 消息
	hasUserMessage := false
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			hasUserMessage = true
			break
		}
	}
	if !hasUserMessage {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no user message found"})
		return
	}

	// 将所有 messages 序列化为 XML 格式作为 prompt
	prompt, err := messagesToXML(req.Messages)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to serialize messages: %v", err)})
		return
	}

	// 检查插件是否连接
	if h.Hub.GetClient() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "extension not connected"})
		return
	}

	// 检查插件端是否空闲
	if !h.Hub.IsExtensionReady() {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": gin.H{
				"message": "extension is busy, please try again later",
				"type":    "rate_limit_error",
			},
		})
		return
	}

	// 生成任务 ID
	taskID := fmt.Sprintf("chatcmpl-%s", uuid.New().String())

	// 存入数据库
	msg := model.Message{
		Role:    "user",
		Content: prompt,
		Status:  "pending",
	}
	h.DB.Create(&msg)

	// 创建任务 channel
	replyCh := h.TaskManager.CreateTask(taskID)
	defer h.TaskManager.RemoveTask(taskID)

	// 构建并发送 WS 指令
	payload, _ := json.Marshal(map[string]string{
		"prompt":          prompt,
		"conversation_id": "",
	})
	wsMsg := &WSMessage{
		ID:      taskID,
		Type:    "CMD_SEND_MESSAGE",
		Payload: payload,
	}

	if err := h.Hub.SendToExtension(wsMsg); err != nil {
		log.Printf("[Chat] send to extension failed: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "extension not connected"})
		return
	}

	// 更新消息状态
	h.DB.Model(&msg).Update("status", "sent")

	modelName := req.Model
	if modelName == "" {
		modelName = "gemini"
	}

	if req.Stream {
		h.handleStream(c, taskID, modelName, replyCh, &msg)
	} else {
		h.handleNonStream(c, taskID, modelName, replyCh, &msg)
	}
}

// handleNonStream 非流式：等待 DONE 后一次性返回
func (h *ChatHandler) handleNonStream(c *gin.Context, taskID, modelName string, replyCh chan *ReplyPayload, msg *model.Message) {
	payload, err := h.TaskManager.WaitForDone(taskID, replyCh, requestTimeout)
	if err != nil {
		log.Printf("[Chat] task failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 更新数据库
	h.DB.Model(msg).Updates(map[string]interface{}{
		"status": "received",
	})
	// 保存 model 回复
	h.DB.Create(&model.Message{
		ConversationID: payload.ConversationID,
		Role:           "model",
		Content:        payload.Text,
		Status:         "received",
	})

	finishReason := "stop"
	resp := ChatResponse{
		ID:      taskID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []Choice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: payload.Text,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: Usage{},
	}

	c.JSON(http.StatusOK, resp)
}

// handleStream 流式：SSE 推送
func (h *ChatHandler) handleStream(c *gin.Context, taskID, modelName string, replyCh chan *ReplyPayload, msg *model.Message) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// 发送第一个 chunk，包含 role
	firstChunk := ChatResponse{
		ID:      taskID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []Choice{
			{
				Index: 0,
				Delta: &ChatMessage{
					Role: "assistant",
				},
				FinishReason: nil,
			},
		},
	}
	writeSSE(c.Writer, flusher, firstChunk)

	timer := time.NewTimer(requestTimeout)
	defer timer.Stop()

	prevText := ""

	for {
		select {
		case payload, ok := <-replyCh:
			if !ok {
				return
			}

			if payload.Status == "ERROR" {
				// 发送错误后结束
				log.Printf("[Chat] stream error: %s", payload.Error)
				return
			}

			// PROCESSING：计算差量并推送增量 chunk
			if payload.Status == "PROCESSING" {
				delta := ""
				if strings.HasPrefix(payload.Text, prevText) {
					delta = payload.Text[len(prevText):]
				} else {
					delta = payload.Text
				}
				prevText = payload.Text

				if delta != "" {
					chunk := ChatResponse{
						ID:      taskID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   modelName,
						Choices: []Choice{
							{
								Index: 0,
								Delta: &ChatMessage{
									Content: delta,
								},
								FinishReason: nil,
							},
						},
					}
					writeSSE(c.Writer, flusher, chunk)
				}
				continue
			}

			if payload.Status == "DONE" {
				// DONE 的 text 是 Markdown 格式（通过复制按钮获取），与之前 PROCESSING 的纯文本不同
				// 不再追加 delta，直接发 finish chunk，避免内容重复
				// 更新数据库
				h.DB.Model(msg).Update("status", "received")
				h.DB.Create(&model.Message{
					ConversationID: payload.ConversationID,
					Role:           "model",
					Content:        payload.Text,
					Status:         "received",
				})

				// 发送 finish chunk
				finishReason := "stop"
				finishChunk := ChatResponse{
					ID:      taskID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   modelName,
					Choices: []Choice{
						{
							Index:        0,
							Delta:        &ChatMessage{},
							FinishReason: &finishReason,
						},
					},
				}
				writeSSE(c.Writer, flusher, finishChunk)

				// 发送 [DONE]
				fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

		case <-timer.C:
			log.Printf("[Chat] stream timeout for task %s", taskID)
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}
