// Package wire_test measures and enforces the network efficiency of the
// server -> client frame stream:
//
//   - TestSpeedChangeBandwidth: speed changes (e.g. dragging the speed slider)
//     must not broadcast a full frame each; they are delivered as tiny state
//     updates instead.
//   - TestSlowClientBackpressure: a client that stops reading must not cause a
//     queue of stale frames to build up; frame acknowledgements keep at most a
//     couple of frames in flight, and the pending frame is replaced by the
//     newest one.
//
// The scenarios deliberately use only stable APIs (and raw bytes for the ack)
// so the exact same file can be run against a pre-change checkout to collect
// "before" numbers. Look for the RESULT lines in verbose test output.
package wire_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/JackWithOneEye/conwaymore/internal/server"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// frameAck is protocol.FrameAck encoded; sent as raw bytes so this file also
// compiles and runs against a checkout that predates the ack protocol (an old
// server simply rejects it as an unknown message, which is harmless).
var frameAck = []byte{3}

type testConfig struct {
	port      uint
	worldSize uint
}

func (c *testConfig) Port() uint      { return c.port }
func (c *testConfig) WorldSize() uint { return c.worldSize }

type testDatabaseConfig struct{ dbUrl string }

func (c *testDatabaseConfig) DBUrl() string { return c.dbUrl }

func newTestServer(t *testing.T, worldSize uint) *httptest.Server {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test_wire_*.db")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	cfg := &testConfig{port: 8080, worldSize: worldSize}
	db := database.NewDatabaseService(&testDatabaseConfig{dbUrl: tmpFile.Name()})
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.NewEngine(cfg, nil, ctx)
	srv := server.NewServer(cfg, db, eng, ctx)
	ts := httptest.NewServer(srv.Handler)

	t.Cleanup(func() {
		cancel()
		ts.Close()
		srv.Close()
		db.Close()
		os.Remove(tmpFile.Name())
	})
	return ts
}

// syncedConn serializes websocket writes (gorilla allows only one concurrent
// writer) so a background reader can ack while the test sends commands.
type syncedConn struct {
	mu sync.Mutex
	c  *websocket.Conn
}

func dial(t *testing.T, ts *httptest.Server) *syncedConn {
	t.Helper()
	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/play"
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return &syncedConn{c: c}
}

func (s *syncedConn) send(msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.c.WriteMessage(websocket.BinaryMessage, msg)
}

// readMsg reads one server message and acks it, as every real client does.
func (s *syncedConn) readMsg(t *testing.T) []byte {
	t.Helper()
	_, msg, err := s.c.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, s.send(frameAck))
	return msg
}

// msgSpeed extracts the speed from either a full Output frame or a lightweight
// state update. A state update starts with the marker 0xff; an Output frame
// starts with the playing flag (0 or 1).
func msgSpeed(t *testing.T, msg []byte) (speed uint16, cells int) {
	t.Helper()
	if len(msg) > 0 && msg[0] > 1 {
		require.GreaterOrEqual(t, len(msg), 4)
		return uint16(msg[2])<<8 | uint16(msg[3]), -1
	}
	var o protocol.Output
	require.NoError(t, o.Decode(msg))
	return o.Speed, int(o.CellsCount)
}

func TestSpeedChangeBandwidth(t *testing.T) {
	ts := newTestServer(t, 256)
	c := dial(t, ts)

	_ = c.readMsg(t) // connect frame

	// Fill the world so that full frames are realistically sized.
	require.NoError(t, c.send((&protocol.Command{Cmd: protocol.Randomise}).Encode()))
	frame := c.readMsg(t)
	_, cells := msgSpeed(t, frame)
	frameSize := len(frame)
	t.Logf("world 256x256 with %d cells: one full frame = %d bytes", cells, frameSize)
	require.Greater(t, frameSize, 100_000, "need realistically sized frames for a meaningful measurement")

	// Simulate a slider drag: many speed changes in quick succession while the
	// game is paused (no other frames are generated).
	const changes = 50
	const finalSpeed = 100 + changes
	for i := 1; i <= changes; i++ {
		require.NoError(t, c.send((&protocol.SetSpeed{Speed: uint16(100 + i)}).Encode()))
	}

	require.NoError(t, c.c.SetReadDeadline(time.Now().Add(30*time.Second)))
	totalBytes, msgs := 0, 0
	for {
		msg := c.readMsg(t)
		msgs++
		totalBytes += len(msg)
		speed, _ := msgSpeed(t, msg)
		if speed == finalSpeed {
			break
		}
	}
	t.Logf("RESULT speed-drag: %d speed changes delivered in %d messages / %d bytes (%.2f%% of a single %d-byte frame)",
		changes, msgs, totalBytes, 100*float64(totalBytes)/float64(frameSize), frameSize)

	assert.Less(t, totalBytes, frameSize,
		"%d speed changes must cost less bandwidth than one full frame", changes)
}

func TestSlowClientBackpressure(t *testing.T) {
	ts := newTestServer(t, 256)
	driver := dial(t, ts)
	slow := dial(t, ts)

	_ = driver.readMsg(t) // connect frame
	_ = slow.readMsg(t)   // connect frame

	// The driver keeps reading and acking in the background so its own
	// connection never throttles the engine.
	go func() {
		for {
			if _, _, err := driver.c.ReadMessage(); err != nil {
				return
			}
			_ = driver.send(frameAck)
		}
	}()

	// Fat frames.
	require.NoError(t, driver.send((&protocol.Command{Cmd: protocol.Randomise}).Encode()))
	for {
		_, cells := msgSpeed(t, slow.readMsg(t))
		if cells > 1000 {
			break
		}
	}

	// Run the game at 100 fps.
	require.NoError(t, driver.send((&protocol.SetSpeed{Speed: 10}).Encode()))
	require.NoError(t, driver.send((&protocol.Command{Cmd: protocol.Play}).Encode()))

	// The slow client now stops reading entirely while ~150 frames go by.
	time.Sleep(1500 * time.Millisecond)

	// Marker: a unique speed that every subsequent full frame carries in its
	// header, so the first message showing it tells us how far behind the slow
	// client was.
	const markerSpeed = 117
	require.NoError(t, driver.send((&protocol.SetSpeed{Speed: markerSpeed}).Encode()))

	require.NoError(t, slow.c.SetReadDeadline(time.Now().Add(30*time.Second)))
	stale, staleBytes := 0, 0
	for {
		msg := slow.readMsg(t)
		speed, _ := msgSpeed(t, msg)
		if speed == markerSpeed {
			break
		}
		stale++
		staleBytes += len(msg)
	}
	t.Logf("RESULT slow-client: after 1.5s of not reading, drained %d stale messages / %d bytes before catching up",
		stale, staleBytes)

	// At most maxInFlight (2) in-flight frames may be stale; the pending frame
	// is always the newest. An unacked queue must never build up.
	assert.LessOrEqual(t, stale, 4, "a slow client must skip stale frames, not drain a queue of them")
}
