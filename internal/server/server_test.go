package server

import (
	"runtime"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

// TestOfferNeverBlocks hammers listener.offer with a concurrent consumer that
// mimics the connection's write pump. A cap-1 "latest wins" channel is prone to
// a deadlock where the sender's select sees a full channel, the consumer drains
// it, and the sender's subsequent drain blocks forever — freezing every client.
func TestOfferNeverBlocks(t *testing.T) {
	l := &listener{msgs: make(chan []byte, 1)}
	frame := (&protocol.Output{Speed: 1}).Encode(nil)
	su := (&protocol.StateUpdate{Speed: 2}).Encode()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-l.msgs:
			default:
				runtime.Gosched()
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 500_000 {
			if i%3 == 0 {
				l.offer(su)
			} else {
				l.offer(frame)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("listener.offer deadlocked")
	}
}
