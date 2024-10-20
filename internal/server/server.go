package server

import (
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
	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

var srv *http.Server

type ServerConfig interface {
	Port() uint
	WorldSize() uint
}

type server struct {
	cfg          ServerConfig
	db           database.DatabaseService
	engine       engine.Engine
	listeners    map[*listener]struct{}
	listenersMtx sync.Mutex
	lastOutput   atomic.Pointer[[]byte]
}

type listener struct {
	msgs chan []byte
}

func NewServer(cfg ServerConfig, db database.DatabaseService, engine engine.Engine) *http.Server {
	if srv != nil {
		return srv
	}

	s := &server{
		cfg:       cfg,
		db:        db,
		engine:    engine,
		listeners: make(map[*listener]struct{}),
	}

	srv = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port()),
		Handler:           s.registerRoutes(),
		IdleTimeout:       time.Minute,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		for {
			o, ok := <-engine.Output()
			if !ok {
				continue
			}
			s.lastOutput.Store(&o)

			s.listenersMtx.Lock()
			for l := range s.listeners {
				select {
				case l.msgs <- o:
				default:
					log.Println("TOO SLOW!!!")
				}
			}
			s.listenersMtx.Unlock()
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
		l.msgs <- *lo
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
		templ.Handler(web.Game("#ffffff", 30, float64(s.engine.Speed()), s.engine.Playing())).ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/play", s.playHandler)

	r.POST("/save", func(c *gin.Context) {
		if lo := s.lastOutput.Load(); lo != nil {
			err := s.db.WriteSeed(c, *lo)
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
	defer s.removeListener(l)

	w := c.Writer
	r := c.Request
	socket, err := websocket.Accept(w, r, nil)

	if err != nil {
		log.Printf("could not open websocket: %s", err)
		_, _ = w.Write([]byte("could not open websocket"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer socket.CloseNow()

	readerMsgChan := make(chan []byte)
	readerErrChan := make(chan error)
	reader := func() {
		_, data, err := socket.Read(c)
		if err != nil {
			readerErrChan <- err
			return
		}
		readerMsgChan <- data
	}

	go reader()

	for {
		select {
		case <-c.Done():
			return
		case payload := <-l.msgs:
			err := socket.Write(c, websocket.MessageBinary, payload)
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
			go reader()
		case err := <-readerErrChan:
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure || websocket.CloseStatus(err) == websocket.StatusGoingAway {
				return
			}
			log.Printf("could not read from websocket: %s", err)
			return
		}
	}
}
