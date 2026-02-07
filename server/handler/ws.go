package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/KodaTao/Gemini-Web-Proxy/server/config"
)

// WebSocket 消息结构
type WSMessage struct {
	ID      string          `json:"id,omitempty"`
	ReplyTo string          `json:"reply_to,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client 表示一个 WebSocket 客户端连接（插件端）
type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	mu     sync.Mutex
	closed bool
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
		c.conn.Close()
	}
}

// Hub 管理 WebSocket 连接
type Hub struct {
	mu     sync.RWMutex
	client *Client
	cfg    *config.WebSocketConfig

	// 消息回调：插件发来的消息通过此 channel 广播
	IncomingMessages chan *WSMessage
}

func NewHub(cfg *config.WebSocketConfig) *Hub {
	return &Hub{
		cfg:              cfg,
		IncomingMessages: make(chan *WSMessage, 100),
	}
}

// GetClient 获取当前连接的客户端
func (h *Hub) GetClient() *Client {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.client
}

// SendToExtension 向插件发送消息
func (h *Hub) SendToExtension(msg *WSMessage) error {
	client := h.GetClient()
	if client == nil {
		return ErrNoClient
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closed {
		return ErrNoClient
	}

	select {
	case client.send <- data:
		return nil
	default:
		return ErrSendBufferFull
	}
}

var (
	ErrNoClient       = &HubError{"no extension client connected"}
	ErrSendBufferFull = &HubError{"send buffer full"}
)

type HubError struct {
	msg string
}

func (e *HubError) Error() string { return e.msg }

// HandleWS 处理 WebSocket 连接请求
func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	// 替换旧连接
	h.mu.Lock()
	if h.client != nil {
		log.Println("[WS] replacing old client connection")
		h.client.Close()
	}
	h.client = client
	h.mu.Unlock()

	log.Println("[WS] extension connected")

	go h.writePump(client)
	go h.pingPump(client)
	h.readPump(client)
}

// readPump 持续读取插件发来的消息
func (h *Hub) readPump(client *Client) {
	defer func() {
		h.mu.Lock()
		if h.client == client {
			h.client = nil
		}
		h.mu.Unlock()
		client.Close()
		log.Println("[WS] extension disconnected")
	}()

	pongTimeout := time.Duration(h.cfg.PongTimeout) * time.Second
	pingInterval := time.Duration(h.cfg.PingInterval) * time.Second

	for {
		_, data, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[WS] read error: %v", err)
			}
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[WS] invalid message: %v", err)
			continue
		}

		// 收到任何消息都刷新读超时（证明连接活跃）
		client.conn.SetReadDeadline(time.Now().Add(pingInterval + pongTimeout))

		// PONG 消息不需要转发
		if msg.Type == "PONG" || msg.Type == "EVENT_PONG" {
			continue
		}

		select {
		case h.IncomingMessages <- &msg:
		default:
			log.Println("[WS] incoming message buffer full, dropping message")
		}
	}
}

// writePump 将消息写入 WebSocket 连接
func (h *Hub) writePump(client *Client) {
	for data := range client.send {
		client.mu.Lock()
		if client.closed {
			client.mu.Unlock()
			return
		}
		err := client.conn.WriteMessage(websocket.TextMessage, data)
		client.mu.Unlock()

		if err != nil {
			log.Printf("[WS] write error: %v", err)
			return
		}
	}
}

// pingPump 定期发送应用层 PING 心跳
// 插件端收到后回复应用层 PONG（JSON 文本），由 readPump 刷新读超时
func (h *Hub) pingPump(client *Client) {
	ticker := time.NewTicker(time.Duration(h.cfg.PingInterval) * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C

		pingMsg := &WSMessage{Type: "PING"}
		data, _ := json.Marshal(pingMsg)

		client.mu.Lock()
		if client.closed {
			client.mu.Unlock()
			return
		}
		err := client.conn.WriteMessage(websocket.TextMessage, data)
		client.mu.Unlock()

		if err != nil {
			log.Printf("[WS] ping error: %v", err)
			return
		}
	}
}
