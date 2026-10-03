// web2api：Gemini 双引擎号池管理网关（Go 实现）
//
// 引擎A：gemini.google.com 网页版原生逆向（Cookie 会话）
// 引擎B：aistudio.google.com 原生逆向（WAA go 后端）或 OpenAI 兼容上游
// 号池管理：SQLite 存储 + Web 管理台（/admin）+ REST 管理 API（/admin/api/*）
//   - API Key 分发 + 按维度用量统计，参考 sub2api 的平台形态。
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"web2api/internal/config"
	"web2api/internal/gateway"
	"web2api/internal/provider"
	"web2api/internal/store"
)

func main() {
	var (
		cfgPath = flag.String("config", "config.yaml", "配置文件路径")
		verbose = flag.Bool("v", false, "详细日志")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "[web2api] ", log.LstdFlags|log.Lmsgprefix)
	if !*verbose {
		logger.SetFlags(0)
	}

	// 1. 加载配置
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Printf("未找到配置文件 %s，使用默认配置", *cfgPath)
			cfg = defaultConfig()
		} else {
			logger.Fatalf("加载配置失败: %v", err)
		}
	}

	// 2. 打开号池存储
	st, err := store.Open(cfg.Server.DBPath)
	if err != nil {
		logger.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	// 3. 构建引擎管理器并初始化
	mgr, err := provider.NewManager(cfg, st)
	if err != nil {
		logger.Fatalf("初始化引擎失败: %v", err)
	}
	initCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := mgr.Init(initCtx); err != nil {
		logger.Printf("[warn] 引擎/号池部分初始化失败（可在管理台补配）: %v", err)
	}

	// 4. 启动网关（含管理台）
	srv, err := gateway.NewServer(mgr, st, cfg.Server.APIKeys, 0, 0, logger)
	if err != nil {
		logger.Fatalf("创建网关失败: %v", err)
	}
	settings, err := st.AdminSettings()
	if err != nil {
		logger.Fatalf("读取初始化状态失败: %v", err)
	}
	if settings == nil {
		logger.Printf("请打开 /admin，设置管理员邮箱、密码、昵称及 Redis 连接信息")
	}

	// 优雅关闭
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Println("收到退出信号，正在关闭...")
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(ctx)
		os.Exit(0)
	}()

	if err := srv.Start(cfg.Server.Listen); err != nil {
		logger.Fatalf("网关启动失败: %v", err)
	}
}

// defaultConfig 无配置文件时的兜底。
func defaultConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Listen:  "0.0.0.0:8800",
			APIKeys: []string{"sk-web2api"},
			DBPath:  "data/web2api.db",
		},
		Routing: config.RoutingConfig{DefaultEngine: "auto"},
		EngineB: config.EngineBConfig{
			Enabled:    true,
			Mode:       "native",
			AuthStates: "auth",
		},
	}
}
