package platform

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NewLogger 按 LOG_LEVEL 构造 JSON 结构化日志。未知级别直接报错，不做静默降级。
func NewLogger(level string) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("未知日志级别 %q，允许 debug/info/warn/error", level)
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})), nil
}
