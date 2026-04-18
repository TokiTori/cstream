package cstream

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewStreamer(t *testing.T) {
	t.Run("default URL", func(t *testing.T) {
		s := NewStreamer("")
		if s.wsURL != DefaultWSURL {
			t.Errorf("expected default URL %s, got %s", DefaultWSURL, s.wsURL)
		}
	})

	t.Run("custom URL", func(t *testing.T) {
		url := "ws://example.com/ws"
		s := NewStreamer(url)
		if s.wsURL != url {
			t.Errorf("expected URL %s, got %s", url, s.wsURL)
		}
	})
}

func TestReportErr(t *testing.T) {
	t.Run("ignore nil error", func(t *testing.T) {
		s := NewStreamer("")
		s.reportErr(nil)
		select {
		case errMsg := <-s.Errors():
			t.Errorf("nil error shouldn't be reported: %v", errMsg)
		default:
		}
		if s.LastError() != nil {
			t.Errorf("lastErr should be nil, got %v", s.LastError())
		}
	})

	t.Run("report non-nil error", func(t *testing.T) {
		s := NewStreamer("")
		errMsg := errors.New("error message here")
		s.reportErr(errMsg)
		select {
		case receivedErr := <-s.Errors():
			if receivedErr != errMsg {
				t.Errorf("expected \"%v\", got \"%v\"", errMsg, receivedErr)
			}
		default:
		}
		if s.LastError() == nil || s.LastError() != errMsg {
			t.Errorf("expected lastErr to be \"%v\", got \"%v\"", errMsg, s.LastError())
		}
	})
}

func TestErrors(t *testing.T) {
	t.Run("no errors on init", func(t *testing.T) {
		s := NewStreamer("")
		select {
		case errMsg := <-s.Errors():
			t.Errorf("unexpected error on init: %v", errMsg)
		default:
		}
	})

	t.Run("stream errors", func(t *testing.T) {
		s := NewStreamer("")
		msgs := make([]error, cap(s.errChan))
		for i := range msgs {
			errMsg := fmt.Errorf("error message here #%d", i)
			msgs[i] = errMsg
			s.reportErr(errMsg)
		}

		for i := range msgs {
			select {
			case receivedErr := <-s.Errors():
				if receivedErr != msgs[i] {
					t.Errorf("expected \"%v\", got \"%v\"", msgs[i], receivedErr)
				}
			default:
				t.Errorf("expected error message in channel, got none")
			}
		}

		// No messages left in channel
		select {
		case errMsg := <-s.Errors():
			t.Errorf("unexpected extra error message: %v", errMsg)
		default:
		}

	})

	t.Run("drops when full", func(t *testing.T) {
		s := NewStreamer("")
		// Fill the channel
		for i := 0; i < cap(s.errChan); i++ {
			s.reportErr(errors.New("fill"))
		}

		// This should not block
		done := make(chan struct{})
		go func() {
			s.reportErr(errors.New("overflow"))
			close(done)
		}()

		select {
		case <-done:
			// OK, did not block
		case <-time.After(100 * time.Millisecond):
			t.Error("reportErr blocked when channel full")
		}
	})
}

func TestNextMsgID(t *testing.T) {
	s := NewStreamer("")
	id1 := s.nextMsgID()
	id2 := s.nextMsgID()
	id3 := s.nextMsgID()

	if id1 != 1 || id2 != 2 || id3 != 3 {
		t.Errorf("expected 1,2,3 got %d,%d,%d", id1, id2, id3)
	}
}
