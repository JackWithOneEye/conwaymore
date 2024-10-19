package livereload

import (
	"bytes"
	"errors"
	"log"
	"net/http"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type livereloadInjectorWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w livereloadInjectorWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func InjectScript(path string, handlerFunc gin.HandlerFunc) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		w := &livereloadInjectorWriter{
			body:           &bytes.Buffer{},
			ResponseWriter: ctx.Writer,
		}
		ctx.Writer = w
		handlerFunc(ctx)
		ctx.Next()
		status := w.Status()
		if status != http.StatusOK {
			return
		}

		b := &bytes.Buffer{}
		err := livereloadScriptTemplate.Execute(b, &livereloadScriptConfig{Path: path, RetryInterval: 500, MaxRetries: 10})
		if err != nil {
			log.Fatalf("could not execute livereload script template: %s", err)
		}

		doc, err := html.Parse(w.body)
		if err != nil {
			log.Fatalf("could not parse response body: %s", err)
		}

		c := b.String()
		err = injectScriptIntoHead(doc.FirstChild, &c)
		if err != nil {
			log.Fatalf("unable to inject livereload script into HTML response body: %s", err)
		}

		w.body.Reset()
		err = html.Render(w.ResponseWriter, doc)
		if err != nil {
			log.Fatalf("could not render modified HTML response: %s", err)
		}
	}
}

func Handler(c *gin.Context) {
	w := c.Writer
	r := c.Request
	socket, err := websocket.Accept(w, r, nil)
	if err != nil {
		log.Printf("could not open livereload websocket: %s", err)
		_, _ = w.Write([]byte("could not open livereload websocket"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer socket.CloseNow()

	ctx := socket.CloseRead(c)

	<-ctx.Done()
}

func injectScriptIntoHead(n *html.Node, content *string) error {
	if n == nil {
		return errors.New("no <head> element node found")
	}

	if n.Type == html.ElementNode && n.Data == "head" {
		scriptNode := &html.Node{
			Type:     html.ElementNode,
			DataAtom: atom.Script,
			Data:     atom.Script.String(),
			FirstChild: &html.Node{
				Type: html.TextNode,
				Data: *content,
			},
		}
		n.AppendChild(scriptNode)
		return nil
	}

	next := n.FirstChild
	if next == nil {
		next = n.NextSibling
	}

	return injectScriptIntoHead(next, content)
}
