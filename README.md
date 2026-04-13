# cstream

Currency streaming library for Binance WebSocket streams.

## Install

```bash
go get github.com/TokiTori/cstream@latest
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/TokiTori/cstream"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Use default Binance URL, or pass a custom one
	streamer := cstream.NewStreamer("")
	go streamer.Start(ctx)

	if err := streamer.Subscribe("btcusdt@trade", "ltcusdt@trade"); err != nil {
		panic(err)
	}

	for m := range streamer.Stream() {
		fmt.Println(m)
	}
}
```

## Error Handling

Errors are reported via a channel and stored for polling:

```go
// Option 1: Drain errors asynchronously
go func() {
	for err := range streamer.Errors() {
		log.Println("stream error:", err)
	}
}()

// Option 2: Check last error (useful when stream closes unexpectedly)
if err := streamer.LastError(); err != nil {
	log.Println("last error:", err)
}
```

Errors include operation context: `connect:`, `read json:`, `write ping:`, `write command:`, `server rejected subscribe:`.

## Graceful Shutdown

```go
streamer.Stop()  // Stops reconnection loop and closes connection
```

`Stop()` is safe to call multiple times. The `Stream()` and `Errors()` channels close when the streamer stops.

## API

| Method | Description |
|--------|-------------|
| `NewStreamer(url)` | Create streamer. Pass `""` for default Binance URL. |
| `Start(ctx)` | Connect and stream. Blocks until ctx canceled or `Stop()` called. |
| `Stop()` | Permanently stop the streamer. |
| `Subscribe(tickers...)` | Subscribe to ticker streams. |
| `Subscriptions()` | Get confirmed subscriptions. |
| `UnsubscribeAll()` | Unsubscribe from all tickers. |
| `Stream()` | Channel of message payloads. |
| `Errors()` | Channel of connection/read/write errors. |
| `LastError()` | Most recent error (useful if errors were dropped). |
