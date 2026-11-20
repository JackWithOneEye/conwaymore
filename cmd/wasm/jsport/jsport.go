//go:build js

// Package jsport sends and receives binary messages on a JavaScript
// MessagePort, the channel between the canvas worker and the engine worker.
package jsport

import "syscall/js"

var uint8Array = js.Global().Get("Uint8Array")

// Port is one end of a MessagePort whose messages are Uint8Arrays.
type Port struct {
	v     js.Value
	msgs  chan []byte
	onMsg js.Func
}

// New starts receiving on the MessagePort v. Received messages wait in a
// buffer of the given size; once it is full, receiving blocks the event loop
// until Messages is read, so size it for the most messages the peer sends
// before waiting for an answer.
func New(v js.Value, buffer int) *Port {
	p := &Port{v: v, msgs: make(chan []byte, buffer)}
	p.onMsg = js.FuncOf(func(this js.Value, args []js.Value) any {
		p.msgs <- FromJS(args[0].Get("data"))
		return nil
	})
	v.Call("addEventListener", "message", p.onMsg)
	v.Call("start")
	return p
}

// Messages returns the messages received on the port, in order.
func (p *Port) Messages() <-chan []byte {
	return p.msgs
}

// Post sends b to the other end of the port.
func (p *Port) Post(b []byte) {
	u := ToJS(b)
	p.v.Call("postMessage", u, []any{u.Get("buffer")})
}

// Close closes the port; the other end receives nothing more from it.
func (p *Port) Close() {
	p.v.Call("removeEventListener", "message", p.onMsg)
	p.v.Call("close")
	p.onMsg.Release()
}

// ToJS copies b into a new Uint8Array.
func ToJS(b []byte) js.Value {
	u := uint8Array.New(len(b))
	js.CopyBytesToJS(u, b)
	return u
}

// FromJS copies the Uint8Array u into a new slice.
func FromJS(u js.Value) []byte {
	b := make([]byte, u.Get("length").Int())
	js.CopyBytesToGo(b, u)
	return b
}
