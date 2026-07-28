package transport

import (
	"context"
	"log/slog"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

type Handler func(protocol.Request) protocol.Response

type Listener interface {
	Serve(ctx context.Context, h Handler) error
	Addr() string
	Close() error
}

type Options struct {
	Logger   *slog.Logger
	Deadline time.Duration
	Grace    time.Duration
}

func (o *Options) applyDefaults() {
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Deadline <= 0 {
		o.Deadline = 5 * time.Second
	}
	if o.Grace <= 0 {
		o.Grace = 5 * time.Second
	}
}
