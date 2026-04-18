package cstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestIntegration_ConnectSubscribeUnsubscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var req map[string]any
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}

			// Send ack for subscribe/unsubscribe
			if method, ok := req["method"].(string); ok {
				id := req["id"]
				if method == "SUBSCRIBE" || method == "UNSUBSCRIBE" {
					resp := map[string]any{"result": nil, "id": id}
					_ = conn.WriteJSON(resp)
				}
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("connect, subscribe, unsubscribe", func(t *testing.T) {
		s := NewStreamer(wsURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		go s.Start(ctx)

		// Wait a bit for connection
		time.Sleep(100 * time.Millisecond)

		// Subscribe
		tickersToSubscribe := []string{"btcusdt@trade", "ltcusdt@trade"}
		err := s.Subscribe(tickersToSubscribe...)
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		// Wait for ack
		time.Sleep(100 * time.Millisecond)
		subs := s.Subscriptions()
		if len(subs) != len(tickersToSubscribe) {
			t.Errorf("expected %v subscriptions, got %v", len(tickersToSubscribe), len(subs))
		}

		// UnsubscribeAll
		s.UnsubscribeAll()
		time.Sleep(100 * time.Millisecond)
		subs = s.Subscriptions()
		if len(subs) != 0 {
			t.Errorf("expected 0 subscriptions, got %v", len(subs))
		}
	})

}

func TestIntegration_ConnectStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}

		msg := map[string]any{
			"stream": "btcusdt@trade",
			"data": map[string]any{
				"s": "BTCUSDT",
				"p": "50000.00",
				"q": "0.001",
			},
		}
		_ = conn.WriteJSON(msg)

		// Keep connection alive
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("connect, subscribe, unsubscribe", func(t *testing.T) {
		s := NewStreamer(wsURL)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		go s.Start(ctx)

		// Wait a bit for connection
		time.Sleep(100 * time.Millisecond)

		select {
		case data := <-s.Stream():
			if data["s"] != "BTCUSDT" {
				t.Errorf("expected symbol BTCUSDT, got %v", data["s"])
			}
		case <-time.After(1 * time.Second):
			t.Error("didn't receive any message")
		}
	})

	t.Run("stop and close", func(t *testing.T) {
		s := NewStreamer(wsURL)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		go s.Start(ctx)

		time.Sleep(100 * time.Millisecond)
		if s.conn == nil {
			t.Errorf("connection should be not nil")
		}
		messagesReceived := 0
	ExitLoop:
		for {
			select {
			case <-time.After(100 * time.Millisecond):
				s.Stop()
				break ExitLoop
			case <-s.Stream():
				messagesReceived++
			}
		}

		if messagesReceived != 1 {
			t.Errorf("expected 1 message, got %v", messagesReceived)
		}
		if s.conn != nil {
			t.Errorf("connection should be nil")
		}
		time.Sleep(100 * time.Millisecond)
		if s.LastError() == nil || !strings.Contains(s.LastError().Error(), "use of closed network connection") {
			t.Errorf("error should contain message \"use of closed network connection\"")
		}

	})

}
