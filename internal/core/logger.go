// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lmittmann/tint"
)

var logger *slog.Logger

// InitLogger initializes the global logger with level and format.
// level: debug, info, warn, error (default: info)
// format: text (colorized tint), json (default: text)
// file: when not nil, every record is also written there as JSON, whatever the console format,
// so the file can be searched and parsed by tools.
func InitLogger(level, format string, file io.Writer) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: slogLevel,
		})
	} else {
		handler = tint.NewHandler(os.Stderr, &tint.Options{
			Level:      slogLevel,
			TimeFormat: "15:04:05",
		})
	}

	if file != nil {
		handler = slog.NewMultiHandler(handler, slog.NewJSONHandler(file, &slog.HandlerOptions{
			Level: slogLevel,
		}))
	}

	logger = slog.New(teeHandler{inner: handler})
	slog.SetDefault(logger)
}

// LogFilePath turns the logfile setting into a path: relative paths live inside the data
// directory, and "" or "off" means no log file.
func LogFilePath(dataDir, setting string) string {
	if setting == "" || setting == "off" {
		return ""
	}
	if filepath.IsAbs(setting) {
		return setting
	}
	if dataDir == "" {
		dataDir = "."
	}
	return filepath.Join(dataDir, setting)
}

// GetLogger returns the global logger
func GetLogger() *slog.Logger {
	if logger == nil {
		InitLogger("info", "text", nil)
	}
	return logger
}

// WithFields creates a logger with the given structured fields
func WithFields(args ...any) *slog.Logger {
	return GetLogger().With(args...)
}

// LogDuration logs the duration of an operation
// Usage: defer LogDuration(logger, "operation_name", time.Now())
func LogDuration(logger *slog.Logger, operation string, start time.Time) {
	duration := time.Since(start)
	logger.Debug("operation completed",
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
		"duration", duration.String(),
	)
}

// WithTool creates a logger with tool execution context
func WithTool(logger *slog.Logger, toolName string, args map[string]any) *slog.Logger {
	return logger.With(
		"tool", toolName,
		"tool_args", args,
	)
}
