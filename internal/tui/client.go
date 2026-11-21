package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/JackWithOneEye/conwaymore/internal/api"
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
	GameID    int64
	GameName  string
	WorldSize uint
}

type saveGameResult struct {
	Err error
}

func connectToAPI(host string, gameID int64) tea.Cmd {
	return func() tea.Msg {
		info, err := resolveGame(host, gameID)
		if err != nil {
			return connectionResult{Conn: nil, Connected: false, Err: err}
		}

		u := url.URL{Scheme: "ws", Host: host, Path: fmt.Sprintf("/games/%d/play", info.ID)}
		conn, _, err := websocket.Dial(context.Background(), u.String(), nil)
		if err != nil {
			return connectionResult{Conn: nil, Connected: false, Err: fmt.Errorf("websocket connection failed: %s", err)}
		}
		// Set read limit to 32MB (same as WASM client)
		conn.SetReadLimit(33554432) // 2^25

		return connectionResult{
			Conn:      conn,
			Connected: true,
			Err:       nil,
			GameID:    info.ID,
			GameName:  info.Name,
			WorldSize: info.WorldSize,
		}
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

// sendAck confirms receipt of a server message so the server keeps at most a
// couple of frames in flight for this client.
func sendAck(conn *websocket.Conn) tea.Cmd {
	return func() tea.Msg {
		msg := &protocol.FrameAck{}
		err := conn.Write(context.Background(), websocket.MessageBinary, msg.Encode())
		if err != nil {
			log.Printf("Error sending frame ack: %v", err)
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

func saveGame(host string, gameID int64) tea.Cmd {
	return func() tea.Msg {
		u := url.URL{Scheme: "http", Host: host, Path: fmt.Sprintf("/api/games/%d/save", gameID)}
		resp, err := http.DefaultClient.Post(u.String(), "", nil)
		if err != nil {
			log.Printf("Error saving game: %v", err)
			return saveGameResult{Err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return saveGameResult{Err: fmt.Errorf("save failed with status %d", resp.StatusCode)}
		}
		return saveGameResult{}
	}
}

func resolveGame(host string, gameID int64) (*api.GameInfo, error) {
	if gameID > 0 {
		u := url.URL{Scheme: "http", Host: host, Path: fmt.Sprintf("/api/games/%d", gameID)}
		resp, err := http.DefaultClient.Get(u.String())
		if err != nil {
			return nil, fmt.Errorf("could not fetch game #%d: %w", gameID, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("game #%d not found", gameID)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d fetching game #%d", resp.StatusCode, gameID)
		}
		d, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		var info api.GameInfo
		if err := json.Unmarshal(d, &info); err != nil {
			return nil, err
		}
		if info.Mode == api.ModeSingle {
			return nil, fmt.Errorf("game #%d is a single-player game, which only runs in the browser", gameID)
		}
		return &info, nil
	}

	u := url.URL{Scheme: "http", Host: host, Path: "/api/games"}
	resp, err := http.DefaultClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("could not list games: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d listing games", resp.StatusCode)
	}
	d, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var list []api.GameInfo
	if err := json.Unmarshal(d, &list); err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Mode != api.ModeSingle {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("no multiplayer games found; create one in the lobby or via POST /api/games")
}
