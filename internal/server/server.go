package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/games"
	"github.com/JackWithOneEye/conwaymore/internal/livereload"
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/JackWithOneEye/conwaymore/internal/web"
	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type ServerConfig interface {
	Port() uint
	WorldSize() uint
}

// StatusGameDeleted is the WebSocket close code sent to viewers when a game is deleted.
const StatusGameDeleted websocket.StatusCode = 4404

type server struct {
	cfg   ServerConfig
	games games.Manager
}

func NewServer(cfg ServerConfig, mgr games.Manager) *http.Server {
	s := &server{
		cfg:   cfg,
		games: mgr,
	}

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port()),
		Handler:           s.registerRoutes(),
		IdleTimeout:       time.Minute,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func (s *server) registerRoutes() http.Handler {
	r := gin.Default()

	assets := r.Group("/assets", func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/assets/fonts/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})
	assets.Static("/", "./internal/web/assets")
	r.GET("/_livereload", livereload.Handler)

	// HTML lobby and game pages
	r.GET("/", livereload.InjectScript("/_livereload", s.lobbyHandler))
	r.GET("/games/list", s.gameListFragmentHandler)
	r.POST("/games", s.createGameFormHandler)
	r.GET("/games/:id", livereload.InjectScript("/_livereload", s.gamePageHandler))
	r.GET("/games/:id/view", s.gameViewHandler)
	r.GET("/games/:id/play", s.playHandler)

	// JSON API
	api := r.Group("/api")
	api.GET("/games", s.apiListGames)
	api.POST("/games", s.apiCreateGame)
	api.GET("/games/:id", s.apiGetGame)
	api.PATCH("/games/:id", s.apiRenameGame)
	api.DELETE("/games/:id", s.apiDeleteGame)
	api.POST("/games/:id/save", s.apiSaveGame)
	api.GET("/games/:id/seed", s.apiGetSeed)
	api.PUT("/games/:id/seed", s.apiPutSeed)

	return r
}

func parseGameID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *server) lobbyHandler(c *gin.Context) {
	list, err := s.games.List(c.Request.Context())
	if err != nil {
		log.Printf("could not list games: %s", err)
		c.String(http.StatusInternalServerError, "could not list games")
		return
	}
	templ.Handler(web.Lobby(list, s.cfg.WorldSize(), "", "", 0, "")).ServeHTTP(c.Writer, c.Request)
}

func (s *server) gameListFragmentHandler(c *gin.Context) {
	list, err := s.games.List(c.Request.Context())
	if err != nil {
		log.Printf("could not list games: %s", err)
		c.String(http.StatusInternalServerError, "could not list games")
		return
	}
	templ.Handler(web.GameList(list)).ServeHTTP(c.Writer, c.Request)
}

func (s *server) createGameFormHandler(c *gin.Context) {
	name := c.PostForm("name")
	mode := games.Mode(c.PostForm("mode"))
	wsRaw := c.PostForm("worldSize")

	var ws uint
	if wsRaw != "" {
		parsed, err := strconv.ParseUint(wsRaw, 10, 32)
		if err == nil {
			ws = uint(parsed)
		}
	}

	info, err := s.games.Create(c.Request.Context(), games.CreateParams{
		Name:      name,
		Mode:      mode,
		WorldSize: ws,
	})
	if err != nil {
		status := http.StatusInternalServerError
		errMsg := "could not create game"
		var verr *games.ValidationError
		switch {
		case errors.As(err, &verr):
			status = http.StatusBadRequest
			errMsg = verr.Message
		case errors.Is(err, games.ErrMaxGamesReached):
			status = http.StatusConflict
			errMsg = err.Error()
		default:
			log.Printf("could not create game: %s", err)
		}

		list, _ := s.games.List(c.Request.Context())
		c.Status(status)
		templ.Handler(web.Lobby(list, s.cfg.WorldSize(), name, mode, ws, errMsg)).ServeHTTP(c.Writer, c.Request)
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/games/%d", info.ID))
}

func (s *server) gamePageHandler(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.Status(http.StatusNotFound)
		templ.Handler(web.GameNotFound("That game does not exist.")).ServeHTTP(c.Writer, c.Request)
		return
	}

	info, err := s.games.GetInfo(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.Status(http.StatusNotFound)
		templ.Handler(web.GameNotFound(fmt.Sprintf("Game #%d was not found.", id))).ServeHTTP(c.Writer, c.Request)
		return
	}
	if err != nil {
		log.Printf("could not get game %d: %s", id, err)
		c.String(http.StatusInternalServerError, "could not load game")
		return
	}

	globals := &web.Globals{
		GameID:    info.ID,
		GameName:  info.Name,
		Mode:      info.Mode,
		WorldSize: info.WorldSize,
	}
	if info.Mode == games.ModeMulti {
		globals.WSPath = fmt.Sprintf("/games/%d/play", info.ID)
	}
	templ.Handler(web.Index(fmt.Sprintf("/games/%d/view", info.ID), globals)).ServeHTTP(c.Writer, c.Request)
}

func (s *server) gameViewHandler(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	info, err := s.games.GetInfo(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("could not get game %d: %s", id, err)
		c.String(http.StatusInternalServerError, "could not load game")
		return
	}

	templ.Handler(web.Game("#ff6600", 30, float64(info.Speed), info.Playing, patterns.Patterns)).ServeHTTP(c.Writer, c.Request)
}

func (s *server) apiListGames(c *gin.Context) {
	list, err := s.games.List(c.Request.Context())
	if err != nil {
		log.Printf("could not list games: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list games"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (s *server) apiCreateGame(c *gin.Context) {
	var params games.CreateParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	info, err := s.games.Create(c.Request.Context(), params)
	if err != nil {
		var verr *games.ValidationError
		switch {
		case errors.As(err, &verr):
			c.JSON(http.StatusBadRequest, verr)
		case errors.Is(err, games.ErrMaxGamesReached):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			log.Printf("could not create game: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create game"})
		}
		return
	}

	c.JSON(http.StatusCreated, info)
}

func (s *server) apiGetGame(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	info, err := s.games.GetInfo(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	if err != nil {
		log.Printf("could not get game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get game"})
		return
	}

	c.JSON(http.StatusOK, info)
}

func (s *server) apiRenameGame(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	info, err := s.games.Rename(c.Request.Context(), id, body.Name)
	if err != nil {
		var verr *games.ValidationError
		switch {
		case errors.As(err, &verr):
			c.JSON(http.StatusBadRequest, verr)
		case errors.Is(err, games.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		default:
			log.Printf("could not rename game %d: %s", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not rename game"})
		}
		return
	}

	c.JSON(http.StatusOK, info)
}

func (s *server) apiDeleteGame(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	err := s.games.Delete(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	if err != nil {
		log.Printf("could not delete game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete game"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *server) apiSaveGame(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	err := s.games.Save(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	if errors.Is(err, games.ErrSinglePlayer) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("could not save game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save game"})
		return
	}

	c.Status(http.StatusOK)
}

func (s *server) apiGetSeed(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	seed, err := s.games.GetSeed(c.Request.Context(), id)
	switch {
	case errors.Is(err, games.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
	case errors.Is(err, games.ErrMultiplayer):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err != nil:
		log.Printf("could not get seed of game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not get game state"})
	default:
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "application/octet-stream", seed)
	}
}

func (s *server) apiPutSeed(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	info, err := s.games.GetInfo(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	if err != nil {
		log.Printf("could not get game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save game"})
		return
	}
	if info.Mode != games.ModeSingle {
		c.JSON(http.StatusConflict, gin.H{"error": games.ErrMultiplayer.Error()})
		return
	}

	// No state of this world encodes to more bytes than a full world.
	limit := int64(protocol.MaxEncodedSize(int(info.WorldSize * info.WorldSize)))
	seed, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "game state is too large"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read game state"})
		return
	}

	err = s.games.SaveSeed(c.Request.Context(), id, seed)
	var verr *games.ValidationError
	switch {
	case errors.As(err, &verr):
		c.JSON(http.StatusBadRequest, verr)
	case errors.Is(err, games.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
	case errors.Is(err, games.ErrMultiplayer):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err != nil:
		log.Printf("could not save seed of game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save game"})
	default:
		c.Status(http.StatusOK)
	}
}

func (s *server) subscribeGame(ctx context.Context, id int64) (games.Game, *games.Subscriber, error) {
	// Retry once if the room closed mid-subscribe due to a concurrent idle unload.
	for attempt := 0; attempt < 2; attempt++ {
		g, err := s.games.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		sub, err := g.Subscribe()
		if errors.Is(err, games.ErrGameClosed) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		return g, sub, nil
	}
	return nil, nil, games.ErrNotFound
}

func (s *server) playHandler(c *gin.Context) {
	id, ok := parseGameID(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	game, sub, err := s.subscribeGame(c.Request.Context(), id)
	if errors.Is(err, games.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	if errors.Is(err, games.ErrMaxLoadedGamesReached) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, games.ErrSinglePlayer) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("could not load game %d: %s", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load game"})
		return
	}
	defer game.Unsubscribe(sub)

	w := c.Writer
	r := c.Request
	socket, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:      websocket.CompressionContextTakeover,
		CompressionThreshold: 1024, // Only compress frames > 1KB
	})
	if err != nil {
		log.Printf("could not open websocket: %s", err)
		return
	}
	defer socket.CloseNow()

	wsCtx, wsCancel := context.WithCancel(c.Request.Context())
	var wg sync.WaitGroup
	wg.Add(1)

	readerMsgChan := make(chan []byte, 10)
	readerErrChan := make(chan error, 1)

	go func() {
		defer func() {
			wg.Done()
			close(readerMsgChan)
			close(readerErrChan)
		}()

		for {
			select {
			case <-wsCtx.Done():
				return
			default:
				_, data, err := socket.Read(wsCtx)
				if err != nil {
					readerErrChan <- err
					return
				}
				select {
				case readerMsgChan <- data:
				case <-wsCtx.Done():
					return
				}
			}
		}
	}()

	defer func() {
		wsCancel()
		wg.Wait()
	}()

	inFlight := 0
	for {
		var outbox <-chan []byte
		if inFlight < protocol.MaxInFlight {
			outbox = sub.Frames()
		}

		select {
		case <-wsCtx.Done():
			return
		case <-sub.Done():
			if sub.Deleted() {
				_ = socket.Close(StatusGameDeleted, "game deleted")
			} else {
				_ = socket.Close(websocket.StatusGoingAway, "server shutting down")
			}
			return
		case payload := <-outbox:
			err := socket.Write(wsCtx, websocket.MessageBinary, payload)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			if err != nil {
				log.Printf("could not write to websocket: %s", err)
				return
			}
			inFlight++
		case msg, ok := <-readerMsgChan:
			if !ok {
				return
			}
			if protocol.IsFrameAck(msg) {
				if inFlight > 0 {
					inFlight--
				}
				continue
			}
			if err := game.Submit(msg); err != nil {
				log.Printf("websocket command produced an error: %s", err)
			}
		case err, ok := <-readerErrChan:
			if !ok {
				return
			}
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			log.Printf("could not read from websocket: %s", err)
			return
		}
	}
}
