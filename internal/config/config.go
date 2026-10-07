// Package config 负责加载 web2api 的 YAML 配置与环境变量覆盖。
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 顶层配置。
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Routing   RoutingConfig   `yaml:"routing"`
	EngineA   EngineAConfig   `yaml:"engine_a"`
	EngineB   EngineBConfig   `yaml:"engine_b"`
	GeminiAPI GeminiAPIConfig `yaml:"gemini_api"`
}

// GeminiAPIConfig configures the official backend independently of web accounts.
// TimeoutSeconds=0 allows long background event streams without a total deadline.
type GeminiAPIConfig struct {
	Enabled        bool   `yaml:"enabled"`
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key"`
	AccessToken    string `yaml:"access_token"`
	Proxy          string `yaml:"proxy"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Listen   string   `yaml:"listen"`   // 监听地址，如 0.0.0.0:8800
	APIKeys  []string `yaml:"api_keys"` // 种子 Key（启动时导入 Key 库，可空）
	DBPath   string   `yaml:"db_path"`  // SQLite 路径（号池/Key/用量）
	LogLevel string   `yaml:"log_level"`
}

// RoutingConfig 模型路由配置。
type RoutingConfig struct {
	DefaultEngine string   `yaml:"default_engine"`  // auto | a | b
	EngineAModels []string `yaml:"engine_a_models"` // 强制走引擎A 的模型名（前缀匹配）
	EngineBModels []string `yaml:"engine_b_models"` // 强制走引擎B 的模型名（前缀匹配）
}

// EngineAConfig 引擎A：gemini.google.com 网页版（原生 Go 逆向）。
type EngineAConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Proxy          string        `yaml:"proxy"` // 可选 http(s)/socks5 代理
	RefreshSeconds int           `yaml:"refresh_seconds"`
	TimeoutSeconds int           `yaml:"timeout_seconds"`
	Accounts       []AccountConf `yaml:"accounts"`
}

// AccountConf 引擎A 的单个账号。
type AccountConf struct {
	Name   string `yaml:"name"`
	PSID   string `yaml:"psid"`   // __Secure-1PSID
	PSIDTS string `yaml:"psidts"` // __Secure-1PSIDTS
}

// EngineBConfig 引擎B：AI Studio。
// mode=native（默认）：内置纯 Go 逆向（WAA go 后端，零浏览器依赖），读取 auth 目录账号。
// mode=upstream：转发到已部署的 OpenAI 兼容服务（如 AIStudio2API）。
type EngineBConfig struct {
	Enabled bool   `yaml:"enabled"`
	Mode    string `yaml:"mode"` // native | upstream

	// native 模式
	AuthStates            string `yaml:"auth_states"` // 账号目录（storage-state.json），支持逗号分隔多路径
	Proxy                 string `yaml:"proxy"`
	RoutingStrategy       string `yaml:"routing_strategy"`  // round-robin | account-sticky
	UpstreamChannels      string `yaml:"upstream_channels"` // playground,build
	PerAccountConcurrency int    `yaml:"per_account_concurrency"`
	InitTimeoutSeconds    int    `yaml:"init_timeout_seconds"`
	RequestTimeoutSeconds int    `yaml:"request_timeout_seconds"`
	TemporaryChat         bool   `yaml:"temporary_chat"`
	RefreshSeconds        int    `yaml:"refresh_seconds"` // 模型目录刷新间隔

	// upstream 模式
	BaseURL     string `yaml:"base_url"`    // 如 http://127.0.0.1:2048
	APIKey      string `yaml:"api_key"`     // 引擎B 的 PROXY_API_KEY
	Passthrough bool   `yaml:"passthrough"` // 是否透传多模态端点
}

// Default 返回无需配置文件即可运行的默认配置。
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:   "0.0.0.0:8800",
			APIKeys:  []string{"sk-web2api"},
			DBPath:   "data/web2api.db",
			LogLevel: "info",
		},
		Routing:   RoutingConfig{DefaultEngine: "auto"},
		GeminiAPI: GeminiAPIConfig{BaseURL: "https://generativelanguage.googleapis.com"},
		EngineA: EngineAConfig{
			Enabled:        true,
			RefreshSeconds: 600,
			TimeoutSeconds: 300,
		},
		EngineB: EngineBConfig{
			Enabled:               true,
			Mode:                  "native",
			AuthStates:            "auth",
			RoutingStrategy:       "round-robin",
			UpstreamChannels:      "playground,build",
			PerAccountConcurrency: 2,
			InitTimeoutSeconds:    120,
			RequestTimeoutSeconds: 300,
			RefreshSeconds:        300,
		},
	}
}

// Load 从文件加载配置，支持环境变量覆盖关键项。配置文件是可选的；
// 文件不存在或被 Docker 错误地挂载成目录时，直接使用默认配置。
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnv(cfg)
			return cfg, nil
		}
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			applyEnv(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	applyEnv(cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	// 环境变量覆盖
	if v := os.Getenv("WEB2API_GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPI.APIKey = v
		cfg.GeminiAPI.Enabled = true
	}
	if v := os.Getenv("WEB2API_GEMINI_ACCESS_TOKEN"); v != "" {
		cfg.GeminiAPI.AccessToken = v
		cfg.GeminiAPI.Enabled = true
	}
	if v := os.Getenv("WEB2API_GEMINI_BASE_URL"); v != "" {
		cfg.GeminiAPI.BaseURL = v
	}
	if v := os.Getenv("WEB2API_GEMINI_PROXY"); v != "" {
		cfg.GeminiAPI.Proxy = v
	}
	if v := os.Getenv("WEB2API_LISTEN"); v != "" {
		cfg.Server.Listen = v
	}
	if v := os.Getenv("WEB2API_API_KEYS"); v != "" {
		cfg.Server.APIKeys = splitCSV(v)
	}
	if v := os.Getenv("WEB2API_DB_PATH"); v != "" {
		cfg.Server.DBPath = v
	}
	if v := os.Getenv("WEB2API_DEFAULT_ENGINE"); v != "" {
		cfg.Routing.DefaultEngine = v
	}
	if v := os.Getenv("WEB2API_ENGINE_B_URL"); v != "" {
		cfg.EngineB.BaseURL = v
	}
	if v := os.Getenv("WEB2API_ENGINE_B_KEY"); v != "" {
		cfg.EngineB.APIKey = v
	}
	if v := os.Getenv("WEB2API_PROXY"); v != "" {
		cfg.EngineA.Proxy = v
	}

	// 默认值
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "0.0.0.0:8800"
	}
	if cfg.Server.DBPath == "" {
		cfg.Server.DBPath = "data/web2api.db"
	}
	if cfg.Routing.DefaultEngine == "" {
		cfg.Routing.DefaultEngine = "auto"
	}
	if cfg.EngineA.RefreshSeconds == 0 {
		cfg.EngineA.RefreshSeconds = 600
	}
	if cfg.EngineA.TimeoutSeconds == 0 {
		cfg.EngineA.TimeoutSeconds = 300
	}
	if cfg.EngineB.BaseURL == "" {
		cfg.EngineB.BaseURL = "http://127.0.0.1:2048"
	}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
