package handler

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// ReplyPayload 解析插件回复的 payload
type ReplyPayload struct {
	Text           string `json:"text"`
	Status         string `json:"status"` // "PROCESSING" | "DONE"
	ConversationID string `json:"conversation_id"`
	Error          string `json:"error,omitempty"`
}

// TaskManager 管理 API 请求与插件回复之间的映射
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]chan *ReplyPayload
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]chan *ReplyPayload),
	}
}

// CreateTask 创建一个任务，返回接收回复的 channel
func (tm *TaskManager) CreateTask(taskID string) chan *ReplyPayload {
	ch := make(chan *ReplyPayload, 10) // 缓冲区支持多次 PROCESSING
	tm.mu.Lock()
	tm.tasks[taskID] = ch
	tm.mu.Unlock()
	return ch
}

// RemoveTask 清理任务
func (tm *TaskManager) RemoveTask(taskID string) {
	tm.mu.Lock()
	if ch, ok := tm.tasks[taskID]; ok {
		close(ch)
		delete(tm.tasks, taskID)
	}
	tm.mu.Unlock()
}

// Dispatch 将插件回复分发到对应的任务 channel
func (tm *TaskManager) Dispatch(msg *WSMessage) {
	if msg.ReplyTo == "" {
		return
	}

	var payload ReplyPayload

	if msg.Type == "EVENT_ERROR" {
		// 解析错误消息
		if msg.Payload != nil {
			json.Unmarshal(msg.Payload, &payload)
		}
		if payload.Error == "" {
			payload.Error = "unknown error from extension"
		}
		payload.Status = "ERROR"
	} else if msg.Type == "EVENT_REPLY" {
		if msg.Payload != nil {
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[TaskManager] invalid reply payload: %v", err)
				return
			}
		}
	} else {
		return
	}

	tm.mu.RLock()
	ch, ok := tm.tasks[msg.ReplyTo]
	tm.mu.RUnlock()

	if !ok {
		log.Printf("[TaskManager] no task found for reply_to=%s", msg.ReplyTo)
		return
	}

	select {
	case ch <- &payload:
	default:
		log.Printf("[TaskManager] task channel full for %s", msg.ReplyTo)
	}
}

// StartDispatcher 启动消息分发协程，从 Hub 的 IncomingMessages 读取并分发
func (tm *TaskManager) StartDispatcher(hub *Hub) {
	go func() {
		for msg := range hub.IncomingMessages {
			log.Printf("[TaskManager] received: type=%s, reply_to=%s", msg.Type, msg.ReplyTo)
			tm.Dispatch(msg)
		}
	}()
}

// WaitForDone 等待任务完成（DONE 或 ERROR），返回最终的完整文本
func (tm *TaskManager) WaitForDone(taskID string, ch chan *ReplyPayload, timeout time.Duration) (*ReplyPayload, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var lastPayload *ReplyPayload
	for {
		select {
		case payload, ok := <-ch:
			if !ok {
				if lastPayload != nil {
					return lastPayload, nil
				}
				return nil, &HubError{"task channel closed unexpectedly"}
			}

			if payload.Status == "ERROR" {
				return payload, &HubError{payload.Error}
			}

			lastPayload = payload

			if payload.Status == "DONE" {
				return payload, nil
			}
			// PROCESSING: 继续等待

		case <-timer.C:
			return nil, &HubError{"task timeout"}
		}
	}
}
