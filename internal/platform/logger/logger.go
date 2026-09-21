package logger

import (
	"log/slog"
	"os"
)

func New(nodeEnv string) *slog.Logger {
	level := slog.LevelInfo
	if nodeEnv == "development" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler).With(
		slog.String("service", "insights-worker"),
		slog.String("env", nodeEnv),
	)
}
