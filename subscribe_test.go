package cstream

import (
	"strings"
	"testing"
)

func TestSubscribe(t *testing.T) {
	t.Run("empty tickers", func(t *testing.T) {
		s := NewStreamer("")
		err := s.Subscribe()
		if err == nil || err.Error() != "no tickers provided" {
			t.Errorf("expected error for empty tickers, got \"%v\"", err)
		}
	})

	t.Run("single ticker", func(t *testing.T) {
		s := NewStreamer("")
		err := s.Subscribe("ticker")
		if err != nil {
			t.Errorf("expected no error for single ticker, got \"%v\"", err)
		}
		select {
		case cmd := <-s.cmdChan:
			if cmd.kind != wsSubscribe {
				t.Errorf("expected wsSubscribe message kind, got \"%v\"", cmd.kind)
			}
			if len(cmd.tickers) != 1 {
				t.Errorf("expected only one ticker, got %d", len(cmd.tickers))
			}
			if cmd.tickers[0] != "ticker" {
				t.Errorf("expected ticker \"ticker\", got \"%s\"", cmd.tickers[0])
			}
		default:
		}
	})

	t.Run("multiple tickers", func(t *testing.T) {
		s := NewStreamer("")
		err := s.Subscribe("ticker1", "ticker2", "ticker3")
		if err != nil {
			t.Errorf("expected no error for multiple tickers, got \"%v\"", err)
		}
	})

	t.Run("command queue full", func(t *testing.T) {
		s := NewStreamer("")
		for i := 0; i < cap(s.cmdChan); i++ {
			err := s.Subscribe("ticker")
			if err != nil {
				t.Fatalf("unexpected error while filling command queue: %v", err)
			}
		}
		err := s.Subscribe("overflow")
		if err == nil || err != ErrCommandQueueFull {
			t.Errorf("expected error for full command queue, got nil")
		}
	})

}

func TestSubscriptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		s := NewStreamer("")
		subscriptions := s.Subscriptions()
		if len(subscriptions) != 0 {
			t.Errorf("expected 0 subscriptions after initialization, got %d", len(subscriptions))
		}

		// It is empty even after Subscribe, because Start has not been called to process the subscribe command
		err := s.Subscribe("ticker1", "ticker2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		subscriptions = s.Subscriptions()
		if len(subscriptions) != 0 {
			t.Errorf("expected 0 subscriptions before processing commands, got %d", len(subscriptions))
		}
	})
}

func TestHandleServerResponse(t *testing.T) {
	t.Run("subscribe/unsubscribe", func(t *testing.T) {
		s := NewStreamer("")
		s.pending[1] = pendingCmd{kind: wsSubscribe, tickers: []string{"btcusdt@trade"}}
		s.handleServerResponse(1, map[string]any{"result": nil, "id": 1})
		subscriptions := s.Subscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("expected 1 subscription after successful subscribe response, got %d", len(subscriptions))
		}

		// Unsubscribing non-subscribed ticker should not cause error or change state
		s.pending[2] = pendingCmd{kind: wsUnsubscribe, tickers: []string{"ltcusdt@trade"}}
		s.handleServerResponse(2, map[string]any{"result": nil, "id": 2})
		subscriptions = s.Subscriptions()
		if len(subscriptions) != 1 {
			t.Errorf("expected 1 subscription after successful unsubscribe response, got %d", len(subscriptions))
		}

		// Unsubscribing already subscribed ticker should remove it
		s.pending[3] = pendingCmd{kind: wsUnsubscribe, tickers: []string{"btcusdt@trade"}}
		s.handleServerResponse(3, map[string]any{"result": nil, "id": 3})
		subscriptions = s.Subscriptions()
		if len(subscriptions) != 0 {
			t.Errorf("expected 0 subscriptions after successful unsubscribe response, got %d", len(subscriptions))
		}
	})

	t.Run("unsubscribe when no tickers subscribed", func(t *testing.T) {
		s := NewStreamer("")
		s.pending[1] = pendingCmd{kind: wsUnsubscribe, tickers: []string{"btcusdt@trade"}}
		s.handleServerResponse(1, map[string]any{"result": nil, "id": 1})
		subscriptions := s.Subscriptions()
		if len(subscriptions) != 0 {
			t.Errorf("expected 0 subscriptions after successful unsubscribe response, got %d", len(subscriptions))
		}
	})

	t.Run("error response", func(t *testing.T) {
		s := NewStreamer("")
		s.pending[1] = pendingCmd{kind: wsSubscribe, tickers: []string{"btcusdt@trade"}}
		s.handleServerResponse(1, map[string]any{"error": "invalid subscription", "id": 1})
		subscriptions := s.Subscriptions()
		if len(subscriptions) != 0 {
			t.Errorf("expected 0 subscriptions after error response, got %d", len(subscriptions))
		}
		select {
		case errMsg := <-s.Errors():
			if !strings.Contains(errMsg.Error(), "server rejected subscribe") {
				t.Errorf("unexpected error message: %v", errMsg)
			}
		default:
			t.Error("expected error message to be sent to errChan")
		}
	})

	t.Run("unknown id ignored", func(t *testing.T) {
		s := NewStreamer("")
		// No panic, just ignored
		s.handleServerResponse(100500, map[string]any{"result": nil, "id": 100500})
		select {
		case errMsg := <-s.Errors():
			t.Errorf("unexpected error message for unknown id: %v", errMsg)
		default:
		}
	})
}
