package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/KodaTao/Gemini-Web-Proxy/server/config"
	"github.com/KodaTao/Gemini-Web-Proxy/server/handler"
	"github.com/KodaTao/Gemini-Web-Proxy/server/model"
)

func main() {
	// 加载配置
	cfg, err := config.Load("../config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("config loaded: port=%d, db=%s", cfg.Server.Port, cfg.Database.Path)

	// 初始化数据库
	db, err := model.InitDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	_ = db // 后续 Step 3 使用
	log.Println("database initialized")

	// 初始化 WebSocket Hub
	hub := handler.NewHub(&cfg.WebSocket)

	// 启动消息处理协程（后续 Step 3 会完善）
	go func() {
		for msg := range hub.IncomingMessages {
			log.Printf("[Hub] received message: type=%s, reply_to=%s", msg.Type, msg.ReplyTo)
		}
	}()

	// 设置路由
	r := gin.Default()
	r.GET("/ws", hub.HandleWS)

	// 启动服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
