package cstream

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Streamer struct {
	wsURL      string
	connMu     sync.RWMutex
	conn       *websocket.Conn
	channel    chan map[string]any
	closeOnce  sync.Once
	errChan    chan error
	errClose   sync.Once
	lastErr    atomic.Pointer[error]
	cmdChan    chan wsCommand
	writeMu    sync.Mutex
	subsMu     sync.RWMutex
	subscribed map[string]struct{}
	pendingMu  sync.Mutex
	pending    map[uint64]pendingCmd
	msgID      uint64
	startOnce  atomic.Bool
	stopMu     sync.Mutex
	stopCancel context.CancelFunc
}

func (s *Streamer) nextMsgID() uint64 {
	return atomic.AddUint64(&s.msgID, 1)
}

func (s *Streamer) reportErr(err error) {
	if err == nil {
		return
	}
	s.lastErr.Store(&err)
	select {
	case s.errChan <- err:
	default:
		// Drop if the consumer isn't draining errors.
	}
}

func (s *Streamer) cleanupConn(conn *websocket.Conn, connCancel context.CancelFunc) {
	connCancel()
	s.connMu.Lock()
	if s.conn == conn {
		s.conn = nil
	}
	s.connMu.Unlock()
	_ = conn.Close()
}

// Default Binance websocket stream URL.
const DefaultWSURL = "wss://stream.binance.com:9443/stream"

// NewStreamer constructs a Streamer. If wsURL is empty, DefaultWSURL is used.
func NewStreamer(wsURL string) *Streamer {
	if wsURL == "" {
		wsURL = DefaultWSURL
	}
	return &Streamer{
		wsURL:      wsURL,
		conn:       nil,
		channel:    make(chan map[string]any, 5),
		errChan:    make(chan error, 5),
		cmdChan:    make(chan wsCommand, 5),
		subscribed: make(map[string]struct{}),
		pending:    make(map[uint64]pendingCmd),
	}
}

// Start connects to the websocket endpoint and begins streaming messages until ctx is canceled,
// Stop() is called, or a read error occurs. Stream() is closed when Start returns.
func (s *Streamer) Start(ctx context.Context) {
	if !s.startOnce.CompareAndSwap(false, true) {
		slog.Warn("Start called more than once; create a new Streamer instance")
		return
	}

	// Create streamer-scoped context derived from parent.
	// Stop() cancels this context to permanently stop the streamer.
	s.stopMu.Lock()
	runCtx, cancel := context.WithCancel(ctx)
	s.stopCancel = cancel
	s.stopMu.Unlock()

	defer s.closeOnce.Do(func() { close(s.channel) })
	defer s.errClose.Do(func() { close(s.errChan) })

	for {
		conn, err := connect(runCtx, s.wsURL)
		if err != nil {
			slog.Error("connect stopped", "err", err)
			s.reportErr(fmt.Errorf("connect: %w", err))
			return
		}

		s.connMu.Lock()
		s.conn = conn
		s.connMu.Unlock()
		connCtx, connCancel := context.WithCancel(runCtx)

		conn.SetPongHandler(func(appData string) error {
			slog.Debug("received pong from server, extending deadline")
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})

		if len(s.Subscriptions()) > 0 {
			slog.Debug("resubscribing to previous tickers", "tickers", s.Subscriptions())
			if err := s.Subscribe(s.Subscriptions()...); err != nil {
				slog.Warn("resubscribe enqueue failed", "err", err)
			}
		}

		go writer(connCtx, conn, s)

		for {
			select {
			case <-runCtx.Done():
				s.cleanupConn(conn, connCancel)
				return
			default:
			}

			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			var message map[string]any
			err := conn.ReadJSON(&message)
			if err != nil {
				slog.Error("error reading JSON", "err", err)
				s.reportErr(fmt.Errorf("read json: %w", err))
				break
			}

			if data, ok := message["data"].(map[string]any); ok {
				select {
				case s.channel <- data:
				case <-runCtx.Done():
					s.cleanupConn(conn, connCancel)
					return
				}
			} else if id, ok := message["id"].(float64); ok {
				// Server response to subscribe/unsubscribe command
				s.handleServerResponse(uint64(id), message)
			}
		}

		s.cleanupConn(conn, connCancel)

		select {
		case <-runCtx.Done():
			return
		case <-time.After(1 * time.Second):
			// Make attempt to reconnect after delay
		}
	}
}

// Stop permanently stops the streamer by canceling its internal context.
// This stops the reconnection loop and closes the underlying websocket connection.
// Stop is safe to call multiple times and before Start.
func (s *Streamer) Stop() {
	s.stopMu.Lock()
	if s.stopCancel != nil {
		s.stopCancel()
	}
	s.stopMu.Unlock()

	s.connMu.Lock()
	conn := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Errors returns an async stream of connection/read/write errors.
// The channel is closed when Start(ctx) returns.
func (s *Streamer) Errors() <-chan error {
	return s.errChan
}

// LastError returns the most recent error reported by the streamer, or nil if none.
// This is useful when errors may have been dropped from the Errors() channel.
func (s *Streamer) LastError() error {
	if p := s.lastErr.Load(); p != nil {
		return *p
	}
	return nil
}

// Stream returns a channel of message payloads.
func (s *Streamer) Stream() <-chan map[string]any {
	return s.channel
}
