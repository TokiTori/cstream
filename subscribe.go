package cstream

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
)

type wsCommandKind uint8

const (
	wsSubscribe wsCommandKind = iota
	wsUnsubscribe
)

type wsCommand struct {
	kind    wsCommandKind
	tickers []string
}

// pendingCmd tracks a command awaiting server acknowledgment.
type pendingCmd struct {
	kind    wsCommandKind
	tickers []string
}

// ErrCommandQueueFull is returned when the command queue is full.
var ErrCommandQueueFull = errors.New("command queue is full; call Start() and drain Stream(), or increase buffering")

func (s *Streamer) enqueueCommand(cmd wsCommand) error {
	select {
	case s.cmdChan <- cmd:
		return nil
	default:
		return ErrCommandQueueFull
	}
}

// Subscribe enqueues a SUBSCRIBE message for the given tickers.
// It returns an error if the internal command queue is full.
func (s *Streamer) Subscribe(tickers ...string) error {
	if len(tickers) == 0 {
		return nil
	}
	return s.enqueueCommand(wsCommand{
		kind:    wsSubscribe,
		tickers: tickers,
	})
}

// Subscriptions returns the set of tickers recorded as subscribed by this Streamer.
func (s *Streamer) Subscriptions() []string {
	s.subsMu.RLock()
	defer s.subsMu.RUnlock()

	items := make([]string, 0, len(s.subscribed))
	for t := range s.subscribed {
		items = append(items, t)
	}
	sort.Strings(items)
	return items
}

// UnsubscribeAll enqueues an UNSUBSCRIBE message for all currently tracked subscriptions.
// It returns an error if the internal command queue is full.
func (s *Streamer) UnsubscribeAll() error {
	return s.enqueueCommand(wsCommand{
		kind:    wsUnsubscribe,
		tickers: s.Subscriptions(),
	})
}

// handleServerResponse processes subscribe/unsubscribe ack from the server.
func (s *Streamer) handleServerResponse(id uint64, message map[string]any) {
	s.pendingMu.Lock()
	cmd, ok := s.pending[id]
	delete(s.pending, id)
	s.pendingMu.Unlock()

	if !ok {
		return // Unknown ID, ignore
	}

	// Check for error response
	if errObj, hasErr := message["error"]; hasErr {
		errMap, _ := errObj.(map[string]any)
		errMsg := "unknown error"
		if m, ok := errMap["msg"].(string); ok {
			errMsg = m
		}
		s.reportErr(fmt.Errorf("server rejected %s: %s", cmdKindString(cmd.kind), errMsg))
		// Don't update subscriptions on error
		return
	}

	// Success - update subscription tracking
	s.subsMu.Lock()
	switch cmd.kind {
	case wsSubscribe:
		for _, t := range cmd.tickers {
			s.subscribed[t] = struct{}{}
		}
		slog.Debug("subscription confirmed", "tickers", cmd.tickers)
	case wsUnsubscribe:
		for _, t := range cmd.tickers {
			delete(s.subscribed, t)
		}
		slog.Debug("unsubscribe confirmed", "tickers", cmd.tickers)
	}
	s.subsMu.Unlock()
}

func cmdKindString(k wsCommandKind) string {
	switch k {
	case wsSubscribe:
		return "subscribe"
	case wsUnsubscribe:
		return "unsubscribe"
	default:
		return "unknown"
	}
}
