package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/JackWithOneEye/conwaymore/cmd/web"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"
)

type wsMessage struct {
	Data []byte
	Err  error
}

type connectionResult struct {
	Conn      *websocket.Conn
	Connected bool
	Err       error
	WorldSize uint
}

type saveGameResult struct {
	Err error
}

func connectToAPI(host string) tea.Cmd {
	return func() tea.Msg {
		worldSize, err := getWorldSize(host)
		if err != nil {
			return connectionResult{Conn: nil, Connected: false, Err: fmt.Errorf("could not get world size: %s", err)}
		}

		u := url.URL{Scheme: "ws", Host: host, Path: "/play"}
		conn, _, err := websocket.Dial(context.Background(), u.String(), nil)
		if err != nil {
			return connectionResult{Conn: nil, Connected: false, Err: fmt.Errorf("websocket connection failed: %s", err)}
		}
		// Set read limit to 32MB (same as WASM client)
		conn.SetReadLimit(33554432) // 2^25

		return connectionResult{Conn: conn, Connected: true, Err: nil, WorldSize: worldSize}
	}
}

func listenForMessages(conn *websocket.Conn) tea.Cmd {
	return func() tea.Msg {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			return wsMessage{Err: err}
		}
		return wsMessage{Data: data}
	}
}

func sendCommand(conn *websocket.Conn, cmd protocol.CommandType) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.Command{Cmd: cmd}
		err := conn.Write(context.Background(), websocket.MessageBinary, msg.Encode())
		if err != nil {
			log.Printf("Error sending command: %v", err)
		}
		return nil
	}
}

func sendCells(conn *websocket.Conn, cells []protocol.Cell) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.SetCells{
			Count: uint16(len(cells)),
			Cells: cells,
		}
		err := conn.Write(context.Background(), websocket.MessageBinary, msg.Encode())
		if err != nil {
			log.Printf("Error sending cells: %v", err)
		}
		return nil
	}
}

func sendSpeed(conn *websocket.Conn, speed uint16) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.SetSpeed{Speed: speed}
		err := conn.Write(context.Background(), websocket.MessageBinary, msg.Encode())
		if err != nil {
			log.Printf("Error sending speed: %v", err)
		}
		return nil
	}
}

func saveGame(host string) tea.Cmd {
	return func() tea.Msg {
		u := url.URL{Scheme: "http", Host: host, Path: "/save"}
		_, err := http.DefaultClient.Post(u.String(), "", nil)
		if err != nil {
			log.Printf("Error saving game: %v", err)
			return saveGameResult{Err: err}
		}
		return saveGameResult{}
	}
}

func processServerMessage(data []byte) ([]protocol.Cell, bool, uint16, error) {
	var output protocol.Output
	err := output.Decode(data)
	if err != nil {
		return nil, false, 0, fmt.Errorf("failed to decode server message: %w", err)
	}

	return output.Cells, output.Playing, output.Speed, nil
}

func getWorldSize(host string) (uint, error) {
	u := url.URL{Scheme: "http", Host: host, Path: "/globals"}
	resp, err := http.DefaultClient.Get(u.String())
	if err != nil {
		return 0, err
	}
	d, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	g := &web.Globals{}
	err = json.Unmarshal(d, g)
	if err != nil {
		return 0, err
	}
	return g.WorldSize, nil
}
