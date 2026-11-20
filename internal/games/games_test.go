package games

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	dbURL       string
	maxGames    int
	maxLoaded   int
	idleTimeout time.Duration
}

func (c testConfig) DBUrl() string              { return c.dbURL }
func (c testConfig) MaxGames() int              { return c.maxGames }
func (c testConfig) MaxLoadedGames() int        { return c.maxLoaded }
func (c testConfig) IdleTimeout() time.Duration { return c.idleTimeout }

func newTestManager(t *testing.T, maxGames, maxLoaded int, idleTimeout time.Duration) (Manager, database.DatabaseService) {
	t.Helper()
	cfg := testConfig{
		dbURL:       filepath.Join(t.TempDir(), "games.db"),
		maxGames:    maxGames,
		maxLoaded:   maxLoaded,
		idleTimeout: idleTimeout,
	}
	db := database.NewDatabaseService(cfg)
	t.Cleanup(func() { db.Close() })
	mgr := NewManager(cfg, db)
	return mgr, db
}

// TestOfferNeverBlocks hammers Subscriber.offer with a concurrent consumer that
// mimics the connection's write pump.
func TestOfferNeverBlocks(t *testing.T) {
	sub := newSubscriber()
	frame := (&protocol.Output{Speed: 1}).Encode(nil)
	su := (&protocol.StateUpdate{Speed: 2}).Encode()

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-sub.Frames():
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
				sub.offer(su)
			} else {
				sub.offer(frame)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Subscriber.offer deadlocked")
	}
}

func TestWorldSizeValidation(t *testing.T) {
	mgr, _ := newTestManager(t, 20, 10, 5*time.Minute)
	defer mgr.Shutdown(context.Background())

	ctx := context.Background()

	for _, ws := range []uint{128, 256, 512, 1024, 2048} {
		info, err := mgr.Create(ctx, CreateParams{Name: "Valid", WorldSize: ws})
		require.NoError(t, err, "worldSize %d should be valid", ws)
		require.Equal(t, ws, info.WorldSize)
	}

	for _, ws := range []uint{0, 16, 64, 127, 129, 300, 1000, 4096, 65536} {
		_, err := mgr.Create(ctx, CreateParams{Name: "Invalid", WorldSize: ws})
		var verr *ValidationError
		require.ErrorAs(t, err, &verr, "worldSize %d should fail validation", ws)
		require.Equal(t, "worldSize", verr.Field)
	}

	_, err := mgr.Create(ctx, CreateParams{Name: "   ", WorldSize: 256})
	var verr *ValidationError
	require.ErrorAs(t, err, &verr)
	require.Equal(t, "name", verr.Field)
}

func TestIdleUnloadAutosavesAndReloads(t *testing.T) {
	mgr, _ := newTestManager(t, 10, 10, 80*time.Millisecond)
	defer mgr.Shutdown(context.Background())

	ctx := context.Background()
	info, err := mgr.Create(ctx, CreateParams{Name: "Idle Test", WorldSize: 128})
	require.NoError(t, err)
	require.False(t, info.Loaded)

	g, err := mgr.Get(ctx, info.ID)
	require.NoError(t, err)
	sub, err := g.Subscribe()
	require.NoError(t, err)
	<-sub.Frames() // initial frame

	// Place a cell and change speed without manually saving.
	sc := &protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 7, Y: 9, Colour: 0x00ffaa}},
	}
	require.NoError(t, g.Submit(sc.Encode()))
	<-sub.Frames()

	require.NoError(t, g.Submit((&protocol.SetSpeed{Speed: 250}).Encode()))
	<-sub.Frames()

	// While subscribed, game stays loaded past the idle timeout.
	time.Sleep(150 * time.Millisecond)
	gotInfo, err := mgr.GetInfo(ctx, info.ID)
	require.NoError(t, err)
	require.True(t, gotInfo.Loaded)
	require.Equal(t, 1, gotInfo.Viewers)

	// Unsubscribe -> game unloads after idle timeout and autosaves.
	g.Unsubscribe(sub)
	require.Eventually(t, func() bool {
		gi, err := mgr.GetInfo(ctx, info.ID)
		return err == nil && !gi.Loaded && gi.SavedAt != nil
	}, 2*time.Second, 20*time.Millisecond)

	// Reload on next Get and verify cell + speed were persisted.
	g2, err := mgr.Get(ctx, info.ID)
	require.NoError(t, err)
	sub2, err := g2.Subscribe()
	require.NoError(t, err)
	defer g2.Unsubscribe(sub2)

	frame := <-sub2.Frames()
	var out protocol.Output
	require.NoError(t, out.Decode(frame))
	require.Equal(t, uint16(250), out.Speed)
	require.Equal(t, uint32(1), out.CellsCount)
	require.Equal(t, uint16(7), out.Cells[0].X)
	require.Equal(t, uint16(9), out.Cells[0].Y)
	require.Equal(t, uint32(0x00ffaa), out.Cells[0].Colour)
}

func TestShutdownAutosavesLoadedGames(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shutdown.db")
	cfg := testConfig{
		dbURL:       dbPath,
		maxGames:    10,
		maxLoaded:   10,
		idleTimeout: 5 * time.Minute,
	}

	db1 := database.NewDatabaseService(cfg)
	mgr1 := NewManager(cfg, db1)
	ctx := context.Background()

	info, err := mgr1.Create(ctx, CreateParams{Name: "Persist Me", WorldSize: 256})
	require.NoError(t, err)

	g, err := mgr1.Get(ctx, info.ID)
	require.NoError(t, err)
	sub, err := g.Subscribe()
	require.NoError(t, err)
	<-sub.Frames()

	sc := &protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 42, Y: 13, Colour: 0x112233}},
	}
	require.NoError(t, g.Submit(sc.Encode()))
	<-sub.Frames()

	// Shut down while client is still connected and without explicit Save.
	require.NoError(t, mgr1.Shutdown(ctx))
	<-sub.Done()
	require.False(t, sub.Deleted())
	require.NoError(t, db1.Close())

	// Re-open database with a fresh manager (simulating server restart).
	db2 := database.NewDatabaseService(cfg)
	defer db2.Close()
	mgr2 := NewManager(cfg, db2)
	defer mgr2.Shutdown(ctx)

	g2, err := mgr2.Get(ctx, info.ID)
	require.NoError(t, err)
	sub2, err := g2.Subscribe()
	require.NoError(t, err)
	defer g2.Unsubscribe(sub2)

	var out protocol.Output
	require.NoError(t, out.Decode(<-sub2.Frames()))
	require.Equal(t, uint32(1), out.CellsCount)
	require.Equal(t, uint16(42), out.Cells[0].X)
	require.Equal(t, uint16(13), out.Cells[0].Y)
	require.Equal(t, uint32(0x112233), out.Cells[0].Colour)
}

func TestDeleteNotifiesActiveSubscribers(t *testing.T) {
	mgr, _ := newTestManager(t, 10, 10, 5*time.Minute)
	defer mgr.Shutdown(context.Background())

	ctx := context.Background()
	info, err := mgr.Create(ctx, CreateParams{Name: "Doomed", WorldSize: 128})
	require.NoError(t, err)

	g, err := mgr.Get(ctx, info.ID)
	require.NoError(t, err)
	sub, err := g.Subscribe()
	require.NoError(t, err)
	<-sub.Frames()

	require.NoError(t, mgr.Delete(ctx, info.ID))

	select {
	case <-sub.Done():
		require.True(t, sub.Deleted())
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber was not notified of deletion")
	}

	_, err = mgr.Get(ctx, info.ID)
	require.ErrorIs(t, err, ErrNotFound)
}
