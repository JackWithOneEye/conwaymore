package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JackWithOneEye/conwaymore/cmd/web"
	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/livereload"
	"github.com/JackWithOneEye/conwaymore/internal/patterns"
	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type ServerConfig interface {
	Port() uint
	WorldSize() uint
}

type server struct {
	cfg          ServerConfig
	db           database.DatabaseService
	engine       engine.Engine
	listeners    map[*listener]struct{}
	listenersMtx sync.RWMutex
	lastOutput   atomic.Value
}

type listener struct {
	msgs chan []byte
}

func NewServer(cfg ServerConfig, db database.DatabaseService, engine engine.Engine, ctx context.Context) *http.Server {
	s := &server{
		cfg:       cfg,
		db:        db,
		engine:    engine,
		listeners: make(map[*listener]struct{}),
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port()),
		Handler:           s.registerRoutes(),
		IdleTimeout:       time.Minute,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case o, ok := <-engine.Output():
				if !ok {
					continue
				}
				s.lastOutput.Store(o)

				s.listenersMtx.RLock()
				for l := range s.listeners {
					select {
					case l.msgs <- o:
					default:
						<-l.msgs
						l.msgs <- o
						log.Println("TOO SLOW!!!")
					}
				}
				s.listenersMtx.RUnlock()
			}
		}
	}()
	go engine.Start()

	return srv
}

func (s *server) addListener(l *listener) {
	s.listenersMtx.Lock()
	defer s.listenersMtx.Unlock()
	s.listeners[l] = struct{}{}
	if lo := s.lastOutput.Load(); lo != nil {
		l.msgs <- lo.([]byte)
	}
}

func (s *server) removeListener(l *listener) {
	s.listenersMtx.Lock()
	defer s.listenersMtx.Unlock()
	delete(s.listeners, l)
}

func (s *server) registerRoutes() http.Handler {
	r := gin.Default()

	r.Static("/assets", "./cmd/web/assets")

	globals := web.Globals{
		WorldSize: s.cfg.WorldSize(),
	}

	r.GET("/_livereload", livereload.Handler)

	r.GET("/", livereload.InjectScript("/_livereload", func(c *gin.Context) {
		templ.Handler(web.Index("/game", &globals)).ServeHTTP(c.Writer, c.Request)
	}))

	r.GET("/game", func(c *gin.Context) {
		templ.Handler(web.Game("#ffffff", 30, float64(s.engine.Speed()), s.engine.Playing(), patterns.Patterns)).ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/globals", func(c *gin.Context) {
		w := c.Writer
		d, err := json.Marshal(globals)
		if err != nil {
			log.Printf("could not marshal globals: %s", err)
			c.String(http.StatusInternalServerError, "error")
			return
		}
		w.Write(d)
		w.Flush()
	})
	//
	// r.GET("/patterns", func(c *gin.Context) {
	// 	w := c.Writer
	// 	d, err := json.Marshal(patterns)
	// 	if err != nil {
	// 		log.Printf("could not marshal patterns: %s", err)
	// 		c.String(http.StatusInternalServerError, "error")
	// 		return
	// 	}
	// 	w.Write(d)
	// 	w.Flush()
	// })

	r.GET("/play", s.playHandler)

	r.POST("/save", func(c *gin.Context) {
		if lo := s.lastOutput.Load(); lo != nil {
			err := s.db.WriteSeed(c, lo.([]byte))
			if err != nil {
				log.Printf("could not save seed: %s", err)
				c.String(http.StatusInternalServerError, "could not save seed")
			}
			return
		}
		c.String(http.StatusInternalServerError, "no seed")
	})

	return r
}

func (s *server) playHandler(c *gin.Context) {
	l := &listener{msgs: make(chan []byte, 4)}
	s.addListener(l)
	defer func() {
		s.removeListener(l)
		close(l.msgs)
	}()

	w := c.Writer
	r := c.Request
	socket, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:      websocket.CompressionContextTakeover,
		CompressionThreshold: 1024, // Only compress frames > 1KB
	})
	if err != nil {
		log.Printf("could not open websocket: %s", err)
		_, _ = w.Write([]byte("could not open websocket"))
		w.WriteHeader(http.StatusInternalServerError)
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

	for {
		select {
		case <-wsCtx.Done():
			return
		case payload := <-l.msgs:
			err := socket.Write(wsCtx, websocket.MessageBinary, payload)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			if err != nil {
				log.Printf("could not write to websocket: %s", err)
				return
			}
		case msg := <-readerMsgChan:
			err = s.engine.SubmitMessage(msg)
			if err != nil {
				log.Printf("websocket command produced an error: %s", err)
			}
		case err := <-readerErrChan:
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			log.Printf("could not read from websocket: %s", err)
			return
		}
	}
}
