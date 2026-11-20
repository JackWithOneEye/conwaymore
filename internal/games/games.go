package games

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
)

const (
	MinWorldSize uint   = 128
	MaxWorldSize uint   = 2048
	MinNameLen   int    = 1
	MaxNameLen   int    = 64
	DefaultSpeed uint16 = 100
)

var ValidWorldSizes = []uint{128, 256, 512, 1024, 2048}

var (
	ErrNotFound              = database.ErrNotFound
	ErrMaxGamesReached       = database.ErrMaxGamesReached
	ErrMaxLoadedGamesReached = errors.New("maximum number of active games reached")
	ErrGameClosed            = errors.New("game closed")
	ErrShuttingDown          = errors.New("server is shutting down")
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"error"`
}

func (e *ValidationError) Error() string {
	return e.Message
}

func IsValidWorldSize(ws uint) bool {
	return ws >= MinWorldSize && ws <= MaxWorldSize && (ws&(ws-1)) == 0
}

type ManagerConfig interface {
	MaxGames() int
	MaxLoadedGames() int
	IdleTimeout() time.Duration
}

type CreateParams struct {
	Name      string `json:"name"`
	WorldSize uint   `json:"worldSize"`
	SpeedMs   uint16 `json:"speedMs,omitempty"`
}

type Info struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	WorldSize uint   `json:"worldSize"`
	Playing   bool   `json:"playing"`
	Speed     uint32 `json:"speed"`
	Viewers   int    `json:"viewers"`
	Loaded    bool   `json:"loaded"`
	CreatedAt int64  `json:"createdAt"`
	SavedAt   *int64 `json:"savedAt"`
}

type Manager interface {
	Create(ctx context.Context, p CreateParams) (Info, error)
	List(ctx context.Context) ([]Info, error)
	GetInfo(ctx context.Context, id int64) (Info, error)
	Get(ctx context.Context, id int64) (Game, error)
	Rename(ctx context.Context, id int64, name string) (Info, error)
	Delete(ctx context.Context, id int64) error
	Save(ctx context.Context, id int64) error
	Shutdown(ctx context.Context) error
}

type Game interface {
	Info() Info
	Subscribe() (*Subscriber, error)
	Unsubscribe(sub *Subscriber)
	Submit(msg []byte) error
}

// Subscriber receives coalesced broadcast frames for a single connected client.
// While delivery is in progress, the pending frame is replaced by newer frames,
// and lightweight StateUpdates are folded into a pending full frame's header so
// neither cells nor the updated state are lost.
type Subscriber struct {
	msgs      chan []byte // capacity 1
	done      chan struct{}
	closeOnce sync.Once
	deleted   atomic.Bool
}

func newSubscriber() *Subscriber {
	return &Subscriber{
		msgs: make(chan []byte, 1),
		done: make(chan struct{}),
	}
}

func (s *Subscriber) Frames() <-chan []byte {
	return s.msgs
}

func (s *Subscriber) Done() <-chan struct{} {
	return s.done
}

func (s *Subscriber) Deleted() bool {
	return s.deleted.Load()
}

func (s *Subscriber) close(deleted bool) {
	if deleted {
		s.deleted.Store(true)
	}
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

// offer hands the newest broadcast message to the subscriber.
//
// Called only by the room's broadcast goroutine while holding room.mu, whose
// consumer only ever receives: after the non-blocking drain the channel is
// empty, so the final send can never block.
func (s *Subscriber) offer(o []byte) {
	var pending []byte
	select {
	case pending = <-s.msgs:
	default:
	}

	if pending != nil && protocol.IsStateUpdate(o) && !protocol.IsStateUpdate(pending) {
		o = protocol.ApplyStateUpdate(pending, o)
	}
	s.msgs <- o
}

type worldSizeConfig uint

func (w worldSizeConfig) WorldSize() uint { return uint(w) }

type room struct {
	id          int64
	worldSize   uint
	createdAt   int64
	eng         engine.Engine
	cancel      context.CancelFunc
	idleTimeout time.Duration
	onIdle      func(*room)

	broadcastDone chan struct{}
	lastOutput    atomic.Value

	mu        sync.RWMutex
	name      string
	savedAt   *int64
	listeners map[*Subscriber]struct{}
	closed    bool
	deleted   bool
	idleTimer *time.Timer
	idleSince time.Time
}

func newRoom(rec *database.GameRecord, idleTimeout time.Duration, onIdle func(*room)) *room {
	roomCtx, cancel := context.WithCancel(context.Background())
	eng := engine.NewEngine(worldSizeConfig(rec.WorldSize), rec.Seed, roomCtx)

	r := &room{
		id:            rec.ID,
		name:          rec.Name,
		worldSize:     rec.WorldSize,
		createdAt:     rec.CreatedAt,
		savedAt:       rec.SavedAt,
		eng:           eng,
		cancel:        cancel,
		idleTimeout:   idleTimeout,
		onIdle:        onIdle,
		broadcastDone: make(chan struct{}),
		listeners:     make(map[*Subscriber]struct{}),
		idleSince:     time.Now(),
	}

	// The engine emits its initial state on creation. Store it before starting
	// so every subscriber and save call immediately observes valid state.
	r.lastOutput.Store(<-eng.Output())

	go r.runBroadcast()
	go eng.Start()

	if idleTimeout > 0 {
		r.idleTimer = time.AfterFunc(idleTimeout, func() {
			r.onIdle(r)
		})
	}

	return r
}

func (r *room) runBroadcast() {
	defer close(r.broadcastDone)
	for o := range r.eng.Output() {
		if protocol.IsStateUpdate(o) {
			if lo := r.lastOutput.Load(); lo != nil {
				r.lastOutput.Store(protocol.ApplyStateUpdate(lo.([]byte), o))
			}
		} else {
			r.lastOutput.Store(o)
		}

		r.mu.RLock()
		for sub := range r.listeners {
			sub.offer(o)
		}
		r.mu.RUnlock()
	}
}

func (r *room) Info() Info {
	r.mu.RLock()
	name := r.name
	viewers := len(r.listeners)
	var savedAt *int64
	if r.savedAt != nil {
		v := *r.savedAt
		savedAt = &v
	}
	r.mu.RUnlock()

	return Info{
		ID:        r.id,
		Name:      name,
		WorldSize: r.worldSize,
		Playing:   r.eng.Playing(),
		Speed:     r.eng.Speed(),
		Viewers:   viewers,
		Loaded:    true,
		CreatedAt: r.createdAt,
		SavedAt:   savedAt,
	}
}

func (r *room) Subscribe() (*Subscriber, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrGameClosed
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	r.idleSince = time.Time{}

	sub := newSubscriber()
	if lo := r.lastOutput.Load(); lo != nil {
		sub.msgs <- lo.([]byte)
	}
	r.listeners[sub] = struct{}{}
	return sub, nil
}

func (r *room) Unsubscribe(sub *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.listeners[sub]; !ok {
		return
	}
	delete(r.listeners, sub)
	if len(r.listeners) == 0 && !r.closed {
		r.idleSince = time.Now()
		if r.idleTimeout > 0 {
			if r.idleTimer != nil {
				r.idleTimer.Stop()
			}
			r.idleTimer = time.AfterFunc(r.idleTimeout, func() {
				r.onIdle(r)
			})
		}
	}
}

func (r *room) Submit(msg []byte) error {
	return r.eng.SubmitMessage(msg)
}

func (r *room) snapshot() []byte {
	if lo := r.lastOutput.Load(); lo != nil {
		return lo.([]byte)
	}
	return nil
}

func (r *room) setName(name string) {
	r.mu.Lock()
	r.name = name
	r.mu.Unlock()
}

func (r *room) setSavedAt(ts int64) {
	r.mu.Lock()
	v := ts
	r.savedAt = &v
	r.mu.Unlock()
}

func (r *room) stop(deleted bool) []byte {
	r.mu.Lock()
	r.closed = true
	if deleted {
		r.deleted = true
	}
	if r.idleTimer != nil {
		r.idleTimer.Stop()
		r.idleTimer = nil
	}
	r.mu.Unlock()

	r.cancel()
	<-r.broadcastDone

	r.mu.Lock()
	for sub := range r.listeners {
		sub.close(deleted)
		delete(r.listeners, sub)
	}
	r.mu.Unlock()

	return r.snapshot()
}

type manager struct {
	db             database.DatabaseService
	maxGames       int
	maxLoadedGames int
	idleTimeout    time.Duration

	mu           sync.Mutex
	rooms        map[int64]*room
	unloading    map[int64]chan struct{}
	shuttingDown bool
}

func NewManager(cfg ManagerConfig, db database.DatabaseService) Manager {
	return &manager{
		db:             db,
		maxGames:       cfg.MaxGames(),
		maxLoadedGames: cfg.MaxLoadedGames(),
		idleTimeout:    cfg.IdleTimeout(),
		rooms:          make(map[int64]*room),
		unloading:      make(map[int64]chan struct{}),
	}
}

func validateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(name)
	if n < MinNameLen || n > MaxNameLen {
		return "", &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("name must be between %d and %d characters", MinNameLen, MaxNameLen),
		}
	}
	return name, nil
}

func infoFromRecord(rec *database.GameRecord) Info {
	playing := false
	speed := uint32(DefaultSpeed)
	if p, s, ok := protocol.PeekHeader(rec.Seed); ok {
		playing = p
		if s > 0 {
			speed = uint32(s)
		}
	}
	var savedAt *int64
	if rec.SavedAt != nil {
		v := *rec.SavedAt
		savedAt = &v
	}
	return Info{
		ID:        rec.ID,
		Name:      rec.Name,
		WorldSize: rec.WorldSize,
		Playing:   playing,
		Speed:     speed,
		Viewers:   0,
		Loaded:    false,
		CreatedAt: rec.CreatedAt,
		SavedAt:   savedAt,
	}
}

func (m *manager) Create(ctx context.Context, p CreateParams) (Info, error) {
	name, err := validateName(p.Name)
	if err != nil {
		return Info{}, err
	}
	if !IsValidWorldSize(p.WorldSize) {
		return Info{}, &ValidationError{
			Field:   "worldSize",
			Message: "world size must be a power of two between 128 and 2048",
		}
	}
	if p.SpeedMs > 1000 {
		return Info{}, &ValidationError{
			Field:   "speedMs",
			Message: "speed must be between 1 and 1000 ms",
		}
	}

	var seed []byte
	if p.SpeedMs > 0 {
		out := &protocol.Output{Playing: false, Speed: p.SpeedMs}
		seed = out.Encode(nil)
	}

	rec, err := m.db.CreateGame(ctx, name, p.WorldSize, seed, m.maxGames)
	if err != nil {
		return Info{}, err
	}
	return infoFromRecord(rec), nil
}

func (m *manager) List(ctx context.Context) ([]Info, error) {
	recs, err := m.db.ListGames(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	loaded := make(map[int64]*room, len(m.rooms))
	for id, r := range m.rooms {
		loaded[id] = r
	}
	m.mu.Unlock()

	result := make([]Info, 0, len(recs))
	for i := range recs {
		if r, ok := loaded[recs[i].ID]; ok {
			result = append(result, r.Info())
		} else {
			result = append(result, infoFromRecord(&recs[i]))
		}
	}
	return result, nil
}

func (m *manager) GetInfo(ctx context.Context, id int64) (Info, error) {
	m.mu.Lock()
	r, ok := m.rooms[id]
	m.mu.Unlock()
	if ok {
		return r.Info(), nil
	}

	rec, err := m.db.GetGame(ctx, id)
	if err != nil {
		return Info{}, err
	}
	return infoFromRecord(rec), nil
}

func (m *manager) Get(ctx context.Context, id int64) (Game, error) {
	for {
		m.mu.Lock()
		if m.shuttingDown {
			m.mu.Unlock()
			return nil, ErrShuttingDown
		}
		if done, ok := m.unloading[id]; ok {
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if r, ok := m.rooms[id]; ok {
			m.mu.Unlock()
			return r, nil
		}

		// Enforce maxLoadedGames by evicting the oldest idle room if needed.
		if m.maxLoadedGames > 0 && len(m.rooms) >= m.maxLoadedGames {
			victim := m.pickIdleVictimLocked()
			if victim == nil {
				m.mu.Unlock()
				return nil, ErrMaxLoadedGamesReached
			}
			done := m.detachForUnloadLocked(victim)
			m.mu.Unlock()
			m.finishUnload(victim, done, "capacity")
			continue
		}

		rec, err := m.db.GetGame(ctx, id)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}

		r := newRoom(rec, m.idleTimeout, m.unloadIfIdle)
		m.rooms[id] = r
		m.mu.Unlock()
		log.Printf("game %d (%s, %dx%d): loaded", r.id, r.name, r.worldSize, r.worldSize)
		return r, nil
	}
}

func (m *manager) pickIdleVictimLocked() *room {
	var oldest *room
	var oldestAt time.Time
	for _, r := range m.rooms {
		r.mu.RLock()
		idle := !r.closed && len(r.listeners) == 0
		since := r.idleSince
		r.mu.RUnlock()
		if !idle {
			continue
		}
		if oldest == nil || since.Before(oldestAt) {
			oldest = r
			oldestAt = since
		}
	}
	return oldest
}

func (m *manager) detachForUnloadLocked(r *room) chan struct{} {
	delete(m.rooms, r.id)
	done := make(chan struct{})
	m.unloading[r.id] = done
	return done
}

func (m *manager) finishUnload(r *room, done chan struct{}, reason string) {
	seed := r.stop(false)
	if len(seed) > 0 {
		now := time.Now().UnixMilli()
		if err := m.db.SaveGameSeed(context.Background(), r.id, seed, now); err != nil {
			if !errors.Is(err, database.ErrNotFound) {
				log.Printf("game %d: autosave on %s unload failed: %v", r.id, reason, err)
			}
		} else {
			log.Printf("game %d: unloaded (%s) and autosaved (%d bytes)", r.id, reason, len(seed))
		}
	} else {
		log.Printf("game %d: unloaded (%s)", r.id, reason)
	}

	m.mu.Lock()
	delete(m.unloading, r.id)
	close(done)
	m.mu.Unlock()
}

func (m *manager) unloadIfIdle(r *room) {
	m.mu.Lock()
	if m.shuttingDown || m.rooms[r.id] != r {
		m.mu.Unlock()
		return
	}
	r.mu.Lock()
	if len(r.listeners) > 0 || r.closed {
		r.mu.Unlock()
		m.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()

	done := m.detachForUnloadLocked(r)
	m.mu.Unlock()

	m.finishUnload(r, done, "idle")
}

func (m *manager) Rename(ctx context.Context, id int64, rawName string) (Info, error) {
	name, err := validateName(rawName)
	if err != nil {
		return Info{}, err
	}
	if err := m.db.RenameGame(ctx, id, name); err != nil {
		return Info{}, err
	}

	m.mu.Lock()
	if r, ok := m.rooms[id]; ok {
		r.setName(name)
		info := r.Info()
		m.mu.Unlock()
		return info, nil
	}
	m.mu.Unlock()

	return m.GetInfo(ctx, id)
}

func (m *manager) Delete(ctx context.Context, id int64) error {
	for {
		m.mu.Lock()
		if done, ok := m.unloading[id]; ok {
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := m.db.DeleteGame(ctx, id); err != nil {
			m.mu.Unlock()
			return err
		}
		r, loaded := m.rooms[id]
		if loaded {
			delete(m.rooms, id)
		}
		m.mu.Unlock()

		if loaded {
			r.stop(true)
			log.Printf("game %d: deleted and stopped", id)
		} else {
			log.Printf("game %d: deleted", id)
		}
		return nil
	}
}

func (m *manager) Save(ctx context.Context, id int64) error {
	for {
		m.mu.Lock()
		if done, ok := m.unloading[id]; ok {
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		r, loaded := m.rooms[id]
		m.mu.Unlock()

		now := time.Now().UnixMilli()
		if loaded {
			seed := r.snapshot()
			if err := m.db.SaveGameSeed(ctx, id, seed, now); err != nil {
				return err
			}
			r.setSavedAt(now)
			return nil
		}

		rec, err := m.db.GetGame(ctx, id)
		if err != nil {
			return err
		}
		return m.db.SaveGameSeed(ctx, id, rec.Seed, now)
	}
}

func (m *manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.shuttingDown = true
	rooms := make([]*room, 0, len(m.rooms))
	for id, r := range m.rooms {
		rooms = append(rooms, r)
		delete(m.rooms, id)
	}
	unloading := make([]chan struct{}, 0, len(m.unloading))
	for _, done := range m.unloading {
		unloading = append(unloading, done)
	}
	m.mu.Unlock()

	for _, done := range unloading {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	var firstErr error
	for _, r := range rooms {
		seed := r.stop(false)
		if len(seed) == 0 {
			continue
		}
		now := time.Now().UnixMilli()
		if err := m.db.SaveGameSeed(ctx, r.id, seed, now); err != nil && !errors.Is(err, database.ErrNotFound) {
			log.Printf("game %d: autosave on shutdown failed: %v", r.id, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			log.Printf("game %d: autosaved on shutdown (%d bytes)", r.id, len(seed))
		}
	}
	return firstErr
}
