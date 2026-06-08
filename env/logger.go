package env

import (
	"io"
	"log/slog"
)

func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	log := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	}))

	return log
}
