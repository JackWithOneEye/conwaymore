//go:build js

package local

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

// TestOverrunningGameLeavesTimeForJavaScript plays a game faster than its
// generations can be computed. In WASM the engine shares one thread with the
// JavaScript event loop, which runs only while every goroutine waits, so the
// engine must keep waiting between generations. Otherwise the engine worker
// never receives the canvas worker's messages: its acks, and commands such as
// Pause.
func TestOverrunningGameLeavesTimeForJavaScript(t *testing.T) {
	l, err := NewGame(1024, nil)
	require.NoError(t, err)
	defer l.Close()
	go func() { // drains frames as the engine worker's delivery loop does
		for range l.Frames() {
		}
	}()

	// A random 1024x1024 world takes far longer than 1 ms per generation.
	require.NoError(t, l.Submit((&protocol.Command{Cmd: protocol.Randomise}).Encode()))
	require.NoError(t, l.Submit((&protocol.SetSpeed{Speed: 1}).Encode()))
	require.NoError(t, l.Submit((&protocol.Command{Cmd: protocol.Play}).Encode()))

	fired := make(chan struct{}, 1)
	cb := js.FuncOf(func(js.Value, []js.Value) any {
		fired <- struct{}{}
		return nil
	})
	defer cb.Release()
	for range 5 {
		js.Global().Call("setTimeout", cb, 10)
		select {
		case <-fired:
		case <-time.After(5 * time.Second):
			t.Fatal("a JavaScript timer did not run within 5 s: the engine kept the thread")
		}
	}
}
