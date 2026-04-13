package cstream

import (
	"context"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

func connect(ctx context.Context, url string) (*websocket.Conn, error) {
	for {
		slog.Debug("connecting", "url", url)
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			slog.Debug("connection established")
			return conn, nil
		}

		slog.Warn("dial error, retrying in 5s", "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}
