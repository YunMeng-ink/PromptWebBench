// Package config 提供环境变量加载与 config 表的带缓存读取。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/promptbench/backend/internal/modelsdev"
)

// Env 是启动时的环境变量配置，进程生命周期内不变。
type Env struct {
	Port string

	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string
	DBSSLMode  string

	JWTSecret         string
	ConfigSecret      string
	AdminInitPassword string
	AdminInitEmail    string

	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	GitHubClientID     string
	GitHubClientSecret string

	// ModelsDevURL 是模型目录地址。默认指向 models.dev 正式地址，
	// 可替换为镜像或本地服务，便于端到端测试与离线部署。
	ModelsDevURL string

	TrustedProxies []string
	StaticDir      string
	LogLevel       string

	// PublicBaseURL 是本服务对外可访问的基地址，用于拼接 GitHub OAuth 回调地址
	// 与登录后落地页地址。前端经反向代理访问后端时应配置为前端同源地址。
	PublicBaseURL string

	// CORSAllowOrigins 与 CookieSecure 供中间件层使用：前者限定允许跨域的前端来源，
	// 后者决定 CSRF 与会话 Cookie 是否要求 HTTPS 传输。
	CORSAllowOrigins []string
	CookieSecure     bool

	// AuthRateLimit 是认证类接口每分钟的限额（按来源地址）。0 表示使用规格默认值 10。
	AuthRateLimit int

	// RateLimit 是全部限流规则的统一上限。0 表示逐条使用规则表中的规格默认值。
	// 用于端到端测试环境：套件从同一地址连续请求，放宽后用例之间不会互相干扰。
	RateLimit int
}

// DSN 组装 pgx 连接串。
func (e Env) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		e.DBHost, e.DBPort, e.DBName, e.DBUser, e.DBPassword, e.DBSSLMode,
	)
}

// Load 读取环境变量。缺少必需项时直接返回错误，不提供隐含默认值。
func Load() (Env, error) {
	e := Env{
		Port:       getenv("PORT", "8080"),
		DBHost:     getenv("DB_HOST", "127.0.0.1"),
		DBPort:     getenv("DB_PORT", "5432"),
		DBName:     getenv("DB_NAME", "prompt_benchmark"),
		DBUser:     getenv("DB_USER", "postgres"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBSSLMode:  getenv("DB_SSLMODE", "disable"),

		JWTSecret:         os.Getenv("JWT_SECRET"),
		ConfigSecret:      os.Getenv("CONFIG_SECRET"),
		AdminInitPassword: os.Getenv("ADMIN_INIT_PASSWORD"),
		AdminInitEmail:    getenv("ADMIN_INIT_EMAIL", "admin@prompt-benchmark.local"),

		SMTPHost:     os.Getenv("SMTP_HOST"),
		SMTPPort:     getenv("SMTP_PORT", "587"),
		SMTPUser:     os.Getenv("SMTP_USER"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     os.Getenv("SMTP_FROM"),

		GitHubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),

		ModelsDevURL: getenv("MODELS_DEV_URL", modelsdev.DefaultURL),

		StaticDir: os.Getenv("STATIC_DIR"),
		LogLevel:  getenv("LOG_LEVEL", "info"),

		PublicBaseURL:    strings.TrimSuffix(getenv("PUBLIC_BASE_URL", "http://localhost:5173"), "/"),
		CORSAllowOrigins: splitNonEmpty(getenv("CORS_ALLOW_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		CookieSecure:     getenv("COOKIE_SECURE", "false") == "true",
	}

	e.TrustedProxies = splitNonEmpty(os.Getenv("TRUSTED_PROXIES"))

	if raw := os.Getenv("AUTH_RATE_LIMIT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return Env{}, fmt.Errorf("AUTH_RATE_LIMIT 必须为正整数，当前值 %q", raw)
		}
		e.AuthRateLimit = limit
	}

	if raw := os.Getenv("RATE_LIMIT"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return Env{}, fmt.Errorf("RATE_LIMIT 必须为正整数，当前值 %q", raw)
		}
		e.RateLimit = limit
	}

	if len(e.JWTSecret) < 32 {
		return Env{}, fmt.Errorf("JWT_SECRET 必须为至少 32 字节的十六进制字符串，当前长度为 %d", len(e.JWTSecret))
	}
	if e.DBName == "" || e.DBUser == "" {
		return Env{}, fmt.Errorf("DB_NAME 与 DB_USER 不可为空")
	}
	return e, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// splitNonEmpty 把逗号分隔的配置拆为切片，忽略空白项。
func splitNonEmpty(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
