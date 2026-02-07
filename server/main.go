package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/KodaTao/Gemini-Web-Proxy/server/config"
	"github.com/KodaTao/Gemini-Web-Proxy/server/handler"
	"github.com/KodaTao/Gemini-Web-Proxy/server/model"
)

func main() {
	// 命令行参数
	configPath := flag.String("c", "", "config.yaml 文件路径 (不指定则使用默认配置)")
	apiKey := flag.String("api-key", "", "API Key，设置后客户端需在 Authorization 头中携带 Bearer <key>")
	flag.Parse()

	// 加载配置
	var cfg *config.Config
	if *configPath != "" {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			log.Fatalf("failed to load config from %s: %v", *configPath, err)
		}
		log.Printf("config loaded from %s", *configPath)
	} else {
		cfg = config.Default()
		log.Println("no config file specified, using default config")
	}

	// 命令行 api-key 优先级高于配置文件
	if *apiKey != "" {
		cfg.APIKey = *apiKey
	}

	// 打印生效配置
	printConfig(cfg)

	// 初始化数据库
	db, err := model.InitDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	log.Println("database initialized")

	// 初始化 WebSocket Hub 和 TaskManager
	hub := handler.NewHub(&cfg.WebSocket)
	taskManager := handler.NewTaskManager()
	taskManager.StartDispatcher(hub)

	// 初始化 ChatHandler
	chatHandler := handler.NewChatHandler(hub, taskManager, db, cfg.APIKey)

	// 设置路由
	gin.SetMode(cfg.Server.Mode)
	r := gin.Default()
	r.GET("/ws", hub.HandleWS)
	r.POST("/v1/chat/completions", chatHandler.Handle)

	// 启动服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func printConfig(cfg *config.Config) {
	fmt.Fprintln(os.Stderr, "========================================")
	fmt.Fprintln(os.Stderr, "  Gemini Web Proxy - Effective Config")
	fmt.Fprintln(os.Stderr, "========================================")
	fmt.Fprintf(os.Stderr, "  Server Port:      %d\n", cfg.Server.Port)
	fmt.Fprintf(os.Stderr, "  Database Path:    %s\n", cfg.Database.Path)
	fmt.Fprintf(os.Stderr, "  WS PingInterval:  %ds\n", cfg.WebSocket.PingInterval)
	fmt.Fprintf(os.Stderr, "  WS PongTimeout:   %ds\n", cfg.WebSocket.PongTimeout)
	if cfg.APIKey != "" {
		fmt.Fprintf(os.Stderr, "  API Key:          %s****\n", cfg.APIKey[:min(4, len(cfg.APIKey))])
	} else {
		fmt.Fprintln(os.Stderr, "  API Key:          (disabled, no auth)")
	}
	fmt.Fprintln(os.Stderr, "========================================")
}
