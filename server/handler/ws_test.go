package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/KodaTao/Gemini-Web-Proxy/server/config"
)

func setupTestHub() (*Hub, *httptest.Server) {
	gin.SetMode(gin.TestMode)

	cfg := &config.WebSocketConfig{
		PingInterval: 2,
		PongTimeout:  5,
	}
	hub := NewHub(cfg)

	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	server := httptest.NewServer(r)

	return hub, server
}

func dialWS(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws failed: %v", err)
	}
	return conn
}

func TestWSConnection(t *testing.T) {
	hub, server := setupTestHub()
	defer server.Close()

	conn := dialWS(t, server)
	defer conn.Close()

	// 等待连接注册
	time.Sleep(100 * time.Millisecond)

	if hub.GetClient() == nil {
		t.Error("expected client to be connected")
	}
}

func TestSendToExtension(t *testing.T) {
	hub, server := setupTestHub()
	defer server.Close()

	conn := dialWS(t, server)
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// Server 向插件发消息
	msg := &WSMessage{
		ID:   "test-123",
		Type: "CMD_SEND_MESSAGE",
	}
	if err := hub.SendToExtension(msg); err != nil {
		t.Fatalf("SendToExtension failed: %v", err)
	}

	// 插件端读取消息
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}

	var received WSMessage
	if err := json.Unmarshal(data, &received); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if received.Type != "CMD_SEND_MESSAGE" {
		t.Errorf("expected type CMD_SEND_MESSAGE, got %s", received.Type)
	}
	if received.ID != "test-123" {
		t.Errorf("expected id test-123, got %s", received.ID)
	}
}

func TestIncomingMessage(t *testing.T) {
	hub, server := setupTestHub()
	defer server.Close()

	conn := dialWS(t, server)
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	// 插件端发送消息
	msg := WSMessage{
		ReplyTo: "task-456",
		Type:    "EVENT_REPLY",
		Payload: json.RawMessage(`{"text":"hello","status":"DONE"}`),
	}
	data, _ := json.Marshal(msg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("write message failed: %v", err)
	}

	// Server 端接收
	select {
	case received := <-hub.IncomingMessages:
		if received.Type != "EVENT_REPLY" {
			t.Errorf("expected type EVENT_REPLY, got %s", received.Type)
		}
		if received.ReplyTo != "task-456" {
			t.Errorf("expected reply_to task-456, got %s", received.ReplyTo)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for incoming message")
	}
}

func TestPingPong(t *testing.T) {
	_, server := setupTestHub()
	defer server.Close()

	conn := dialWS(t, server)
	defer conn.Close()

	// 等待收到 PING 消息（PingInterval=2s）
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ping failed: %v", err)
	}

	var msg WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if msg.Type != "PING" {
		t.Errorf("expected PING, got %s", msg.Type)
	}

	// 回复 PONG
	pong := WSMessage{Type: "PONG"}
	pongData, _ := json.Marshal(pong)
	if err := conn.WriteMessage(websocket.TextMessage, pongData); err != nil {
		t.Fatalf("write pong failed: %v", err)
	}
}

func TestSendToExtensionNoClient(t *testing.T) {
	cfg := &config.WebSocketConfig{PingInterval: 30, PongTimeout: 10}
	hub := NewHub(cfg)

	err := hub.SendToExtension(&WSMessage{Type: "test"})
	if err != ErrNoClient {
		t.Errorf("expected ErrNoClient, got %v", err)
	}
}

func TestReplaceOldConnection(t *testing.T) {
	hub, server := setupTestHub()
	defer server.Close()

	// 第一个连接
	conn1 := dialWS(t, server)
	time.Sleep(100 * time.Millisecond)

	// 第二个连接应替换第一个
	conn2 := dialWS(t, server)
	time.Sleep(100 * time.Millisecond)
	defer conn2.Close()

	// conn1 应该已被关闭
	conn1.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err := conn1.ReadMessage()
	if err == nil {
		t.Error("expected old connection to be closed")
	}

	// 新连接应该可以正常通信
	msg := &WSMessage{Type: "test"}
	if err := hub.SendToExtension(msg); err != nil {
		t.Errorf("send to new client failed: %v", err)
	}
}
