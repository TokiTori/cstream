package cstream

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

func writer(ctx context.Context, conn *websocket.Conn, s *Streamer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.writeMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			s.writeMu.Unlock()
			if err != nil {
				slog.Error("error sending ping", "err", err)
				s.reportErr(fmt.Errorf("write ping: %w", err))
				_ = conn.Close()
				return
			}
			slog.Debug("ping sent to server")
		case wsCommand := <-s.cmdChan:
			msgID := s.nextMsgID()
			var msg map[string]any
			switch wsCommand.kind {
			case wsSubscribe:
				msg = map[string]any{
					"method": "SUBSCRIBE",
					"params": wsCommand.tickers,
					"id":     msgID,
				}
			case wsUnsubscribe:
				msg = map[string]any{
					"method": "UNSUBSCRIBE",
					"params": wsCommand.tickers,
					"id":     msgID,
				}
			}
			// Track pending command before sending
			s.pendingMu.Lock()
			s.pending[msgID] = pendingCmd{kind: wsCommand.kind, tickers: wsCommand.tickers}
			s.pendingMu.Unlock()

			s.writeMu.Lock()
			err := conn.WriteJSON(msg)
			if err != nil {
				slog.Error("error writing command", "err", err)
				s.reportErr(fmt.Errorf("write command: %w", err))
				s.writeMu.Unlock()
				// Remove from pending on write failure
				s.pendingMu.Lock()
				delete(s.pending, msgID)
				s.pendingMu.Unlock()
				_ = conn.Close()
				return
			}
			s.writeMu.Unlock()
			slog.Debug("command sent", "method", msg["method"], "tickers", wsCommand.tickers, "id", msgID)
		case <-ctx.Done():
			return
		}
	}
}
