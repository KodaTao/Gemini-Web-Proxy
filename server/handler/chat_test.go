package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/KodaTao/Gemini-Web-Proxy/server/config"
	"github.com/KodaTao/Gemini-Web-Proxy/server/model"
)

func setupChatTest(t *testing.T) (*Hub, *TaskManager, *httptest.Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &config.WebSocketConfig{PingInterval: 60, PongTimeout: 10}
	hub := NewHub(cfg)
	tm := NewTaskManager()
	tm.StartDispatcher(hub)

	tmpDir := t.TempDir()
	db, err := model.InitDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	chatHandler := NewChatHandler(hub, tm, db, "")

	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	r.POST("/v1/chat/completions", chatHandler.Handle)

	server := httptest.NewServer(r)
	return hub, tm, server, r
}

// 模拟插件：连接 WS，接收 CMD_SEND_MESSAGE，回复 EVENT_REPLY
func simulateExtension(t *testing.T, server *httptest.Server, replies []WSMessage) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws failed: %v", err)
	}

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			if msg.Type == "CMD_SEND_MESSAGE" {
				// 用收到的任务 ID 回复
				for _, reply := range replies {
					r := reply
					r.ReplyTo = msg.ID
					rData, _ := json.Marshal(r)
					conn.WriteMessage(websocket.TextMessage, rData)
					time.Sleep(50 * time.Millisecond)
				}
			}
		}
	}()

	return conn
}

func TestNonStreamChat(t *testing.T) {
	_, _, server, r := setupChatTest(t)
	defer server.Close()

	// 模拟插件回复
	donePayload, _ := json.Marshal(map[string]string{
		"text":            "Hello from Gemini!",
		"status":          "DONE",
		"conversation_id": "conv-123",
	})
	replies := []WSMessage{
		{Type: "EVENT_REPLY", Payload: donePayload},
	}
	conn := simulateExtension(t, server, replies)
	defer conn.Close()

	time.Sleep(200 * time.Millisecond) // 等待 WS 连接注册

	// 发送 API 请求
	reqBody := `{"model":"gemini","messages":[{"role":"user","content":"Hello"}],"stream":false}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}

	if resp.Object != "chat.completion" {
		t.Errorf("expected object chat.completion, got %s", resp.Object)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message == nil {
		t.Fatal("expected message in choice")
	}
	if resp.Choices[0].Message.Content != "Hello from Gemini!" {
		t.Errorf("expected 'Hello from Gemini!', got '%s'", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", resp.Choices[0].Message.Role)
	}
	if *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", *resp.Choices[0].FinishReason)
	}
}

func TestStreamChat(t *testing.T) {
	_, _, server, r := setupChatTest(t)
	defer server.Close()

	// 模拟插件回复：先 PROCESSING，再 DONE
	p1, _ := json.Marshal(map[string]string{
		"text":            "Hello",
		"status":          "PROCESSING",
		"conversation_id": "conv-456",
	})
	p2, _ := json.Marshal(map[string]string{
		"text":            "Hello from Gemini!",
		"status":          "DONE",
		"conversation_id": "conv-456",
	})
	replies := []WSMessage{
		{Type: "EVENT_REPLY", Payload: p1},
		{Type: "EVENT_REPLY", Payload: p2},
	}
	conn := simulateExtension(t, server, replies)
	defer conn.Close()

	time.Sleep(200 * time.Millisecond)

	reqBody := `{"model":"gemini","messages":[{"role":"user","content":"Hello"}],"stream":true}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 解析 SSE 事件
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	var chunks []ChatResponse
	gotDone := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			gotDone = true
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var chunk ChatResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				t.Fatalf("unmarshal chunk failed: %v", err)
			}
			chunks = append(chunks, chunk)
		}
	}

	if !gotDone {
		t.Error("expected data: [DONE] in stream")
	}

	// 至少应该有：role chunk + content chunk(s) + finish chunk
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}

	// 第一个 chunk 应该有 role
	if chunks[0].Choices[0].Delta.Role != "assistant" {
		t.Errorf("first chunk should have role 'assistant'")
	}

	// 最后一个 chunk 应该有 finish_reason
	lastChunk := chunks[len(chunks)-1]
	if lastChunk.Choices[0].FinishReason == nil || *lastChunk.Choices[0].FinishReason != "stop" {
		t.Error("last chunk should have finish_reason 'stop'")
	}

	// 所有 chunk 的 object 应该是 chat.completion.chunk
	for _, c := range chunks {
		if c.Object != "chat.completion.chunk" {
			t.Errorf("expected object chat.completion.chunk, got %s", c.Object)
		}
	}
}

func TestChatNoExtension(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.WebSocketConfig{PingInterval: 60, PongTimeout: 10}
	hub := NewHub(cfg)
	tm := NewTaskManager()
	tm.StartDispatcher(hub)

	tmpDir := t.TempDir()
	db, _ := model.InitDB(filepath.Join(tmpDir, "test.db"))

	chatHandler := NewChatHandler(hub, tm, db, "")

	r := gin.New()
	r.POST("/v1/chat/completions", chatHandler.Handle)

	reqBody := `{"model":"gemini","messages":[{"role":"user","content":"Hello"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestChatNoUserMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.WebSocketConfig{PingInterval: 60, PongTimeout: 10}
	hub := NewHub(cfg)
	tm := NewTaskManager()

	tmpDir := t.TempDir()
	db, _ := model.InitDB(filepath.Join(tmpDir, "test.db"))

	chatHandler := NewChatHandler(hub, tm, db, "")

	r := gin.New()
	r.POST("/v1/chat/completions", chatHandler.Handle)

	reqBody := `{"model":"gemini","messages":[{"role":"system","content":"You are helpful"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatAPIKeyAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.WebSocketConfig{PingInterval: 60, PongTimeout: 10}
	hub := NewHub(cfg)
	tm := NewTaskManager()

	tmpDir := t.TempDir()
	db, _ := model.InitDB(filepath.Join(tmpDir, "test.db"))

	chatHandler := NewChatHandler(hub, tm, db, "my-secret-key")

	r := gin.New()
	r.POST("/v1/chat/completions", chatHandler.Handle)

	reqBody := `{"model":"gemini","messages":[{"role":"user","content":"Hello"}]}`

	// 无 Authorization 头 → 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth header, got %d", w.Code)
	}

	// 错误的 key → 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong key, got %d", w.Code)
	}

	// 正确的 key → 应该通过认证（会因为无插件返回 503）
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer my-secret-key")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 with correct key (no extension), got %d", w.Code)
	}
}

func TestTaskManagerDispatch(t *testing.T) {
	tm := NewTaskManager()

	taskID := "test-task-1"
	ch := tm.CreateTask(taskID)

	// 模拟分发
	payload, _ := json.Marshal(map[string]string{
		"text":   "response",
		"status": "DONE",
	})
	tm.Dispatch(&WSMessage{
		ReplyTo: taskID,
		Type:    "EVENT_REPLY",
		Payload: payload,
	})

	select {
	case reply := <-ch:
		if reply.Text != "response" {
			t.Errorf("expected text 'response', got '%s'", reply.Text)
		}
		if reply.Status != "DONE" {
			t.Errorf("expected status DONE, got %s", reply.Status)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for dispatch")
	}

	tm.RemoveTask(taskID)
}
