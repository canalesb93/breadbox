package cli

import (
	"log/slog"
	"os"
	"strings"

	"breadbox/internal/config"
)

// newLogger mirrors the helper previously in cmd/breadbox/main.go. Each
// long-running command (serve, mcp) calls this with the loaded config so
// LOG_LEVEL + Environment are honored consistently.
func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Environment != "docker" {
		level = slog.LevelDebug
	}

	if cfg.LogLevel != "" {
		switch strings.ToLower(cfg.LogLevel) {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}

	var handler slog.Handler
	if cfg.Environment == "docker" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	logger := slog.New(handler)

	if cfg.LogLevel != "" {
		valid := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !valid[strings.ToLower(cfg.LogLevel)] {
			logger.Warn("invalid LOG_LEVEL value, using default", "log_level", cfg.LogLevel, "default", level.String())
		}
	}

	return logger
}
