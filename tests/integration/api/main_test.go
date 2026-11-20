package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/games"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/JackWithOneEye/conwaymore/internal/server"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
)

type APITestSuite struct {
	suite.Suite
	server  *http.Server
	manager games.Manager
	db      database.DatabaseService
	dbFile  string
}

// ackFrame confirms receipt of a server message, as every real client does.
func ackFrame(c *websocket.Conn) error {
	return c.WriteMessage(websocket.BinaryMessage, (&protocol.FrameAck{}).Encode())
}

type testConfig struct {
	dbURL       string
	port        uint
	worldSize   uint
	maxGames    int
	maxLoaded   int
	idleTimeout time.Duration
}

func (c *testConfig) DBUrl() string              { return c.dbURL }
func (c *testConfig) Port() uint                 { return c.port }
func (c *testConfig) WorldSize() uint            { return c.worldSize }
func (c *testConfig) MaxGames() int              { return c.maxGames }
func (c *testConfig) MaxLoadedGames() int        { return c.maxLoaded }
func (c *testConfig) IdleTimeout() time.Duration { return c.idleTimeout }

func (suite *APITestSuite) SetupTest() {
	tmpFile, err := os.CreateTemp("", "test_integration_*.db")
	suite.Require().NoError(err)
	dbFile := tmpFile.Name()
	tmpFile.Close()

	cfg := &testConfig{
		dbURL:       dbFile,
		port:        8080,
		worldSize:   1024,
		maxGames:    5,
		maxLoaded:   5,
		idleTimeout: 5 * time.Minute,
	}
	db := database.NewDatabaseService(cfg)
	mgr := games.NewManager(cfg, db)
	suite.server = server.NewServer(cfg, mgr)
	suite.manager = mgr
	suite.db = db
	suite.dbFile = dbFile
}

func (suite *APITestSuite) TearDownTest() {
	_ = suite.manager.Shutdown(context.Background())
	suite.server.Close()
	suite.db.Close()
	if suite.dbFile != "" {
		os.Remove(suite.dbFile)
	}
}

func (suite *APITestSuite) createGameAPI(name string, worldSize uint) games.Info {
	payload, err := json.Marshal(games.CreateParams{Name: name, WorldSize: worldSize})
	suite.Require().NoError(err)
	req := httptest.NewRequest("POST", "/api/games", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Require().Equal(http.StatusCreated, w.Code, w.Body.String())

	var info games.Info
	suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &info))
	return info
}

func (suite *APITestSuite) dialGame(ts *httptest.Server, id int64) *websocket.Conn {
	u, err := url.Parse(ts.URL)
	suite.Require().NoError(err)
	u.Scheme = "ws"
	u.Path = fmt.Sprintf("/games/%d/play", id)

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	suite.Require().NoError(err)
	return c
}

func (suite *APITestSuite) TestLobbyAndHealthCheck() {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)
	suite.Contains(w.Body.String(), "Create game")
	for _, ws := range []string{"128", "256", "512", "1024", "2048"} {
		suite.Contains(w.Body.String(), fmt.Sprintf(`value="%s"`, ws))
	}
}

func (suite *APITestSuite) TestCreateValidationAndLimits() {
	// Invalid world sizes rejected with 400 + field.
	for _, ws := range []uint{64, 192, 300, 4096} {
		payload, _ := json.Marshal(games.CreateParams{Name: "BadSize", WorldSize: ws})
		req := httptest.NewRequest("POST", "/api/games", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		suite.server.Handler.ServeHTTP(w, req)
		suite.Equal(http.StatusBadRequest, w.Code)
		suite.Contains(w.Body.String(), `"field":"worldSize"`)
	}

	// Valid powers of two 128..2048 all succeed (5 games = maxGames).
	for i, ws := range []uint{128, 256, 512, 1024, 2048} {
		info := suite.createGameAPI(fmt.Sprintf("World %d", ws), ws)
		suite.Equal(int64(i+1), info.ID)
		suite.Equal(ws, info.WorldSize)
	}

	// 6th game hits max_games limit -> 409 Conflict.
	payload, _ := json.Marshal(games.CreateParams{Name: "Overflow", WorldSize: 256})
	req := httptest.NewRequest("POST", "/api/games", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusConflict, w.Code)
}

func (suite *APITestSuite) TestLobbyFormCreateRedirectsToGame() {
	form := url.Values{}
	form.Set("name", "Form Created")
	form.Set("worldSize", "512")

	req := httptest.NewRequest("POST", "/games", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)

	suite.Equal(http.StatusSeeOther, w.Code)
	suite.Equal("/games/1", w.Header().Get("Location"))

	// Game page and view fragment render cleanly.
	reqPage := httptest.NewRequest("GET", "/games/1", nil)
	wPage := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wPage, reqPage)
	suite.Equal(http.StatusOK, wPage.Code)
	suite.Contains(wPage.Body.String(), "Form Created")
	suite.Contains(wPage.Body.String(), "/games/1/play")

	reqView := httptest.NewRequest("GET", "/games/1/view", nil)
	wView := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wView, reqView)
	suite.Equal(http.StatusOK, wView.Code)
	suite.Contains(wView.Body.String(), "PLAY")
	suite.Contains(wView.Body.String(), "100 ms")
}

func (suite *APITestSuite) TestPlayAndSaveGameState() {
	info := suite.createGameAPI("Save Test", 256)
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	c := suite.dialGame(ts, info.ID)
	defer c.Close()

	_, _, err := c.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(c))

	setCells := protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 0, Y: 0, Colour: 0xff0000, Age: 0}},
	}
	suite.Require().NoError(c.WriteMessage(websocket.BinaryMessage, setCells.Encode()))

	_, msg, err := c.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(c))
	var decodedOutput protocol.Output
	suite.Require().NoError(decodedOutput.Decode(msg))

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/games/%d/save", info.ID), nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)

	rec, err := suite.db.GetGame(context.Background(), info.ID)
	suite.Require().NoError(err)
	suite.NotEmpty(rec.Seed)

	var output protocol.Output
	suite.Require().NoError(output.Decode(rec.Seed))
	suite.Equal(uint32(1), output.CellsCount)
	suite.Equal(uint16(0), output.Cells[0].X)
	suite.Equal(uint16(0), output.Cells[0].Y)
	suite.Equal(uint32(0xff0000), output.Cells[0].Colour)
}

func (suite *APITestSuite) TestGliderPattern() {
	info := suite.createGameAPI("Glider", 1024)
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	c := suite.dialGame(ts, info.ID)
	defer c.Close()

	_, _, err := c.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(c))

	gliderCells := []protocol.Cell{
		{X: 11, Y: 10, Colour: 0x00ff00, Age: 0},
		{X: 12, Y: 11, Colour: 0x00ff00, Age: 0},
		{X: 10, Y: 12, Colour: 0x00ff00, Age: 0},
		{X: 11, Y: 12, Colour: 0x00ff00, Age: 0},
		{X: 12, Y: 12, Colour: 0x00ff00, Age: 0},
	}
	setCells := protocol.SetCells{
		Count: uint16(len(gliderCells)),
		Cells: gliderCells,
	}
	suite.Require().NoError(c.WriteMessage(websocket.BinaryMessage, setCells.Encode()))

	_, msg, err := c.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(c))
	var decodedOutput protocol.Output
	suite.Require().NoError(decodedOutput.Decode(msg))

	for range 5 {
		nextCmd := protocol.Command{Cmd: protocol.Next}
		suite.Require().NoError(c.WriteMessage(websocket.BinaryMessage, nextCmd.Encode()))
		_, msg, err = c.ReadMessage()
		suite.Require().NoError(err)
		suite.Require().NoError(ackFrame(c))
		suite.Require().NoError(decodedOutput.Decode(msg))
	}

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/games/%d/save", info.ID), nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)

	rec, err := suite.db.GetGame(context.Background(), info.ID)
	suite.Require().NoError(err)

	var output protocol.Output
	suite.Require().NoError(output.Decode(rec.Seed))
	suite.Equal(uint32(5), output.CellsCount)

	expectedCells := []protocol.Cell{
		{X: 11, Y: 12, Colour: 0x00ff00, Age: 0},
		{X: 12, Y: 13, Colour: 0x00ff00, Age: 3},
		{X: 12, Y: 14, Colour: 0x00ff00, Age: 0},
		{X: 13, Y: 12, Colour: 0x00ff00, Age: 2},
		{X: 13, Y: 13, Colour: 0x00ff00, Age: 1},
	}
	sort.Slice(output.Cells, func(i, j int) bool {
		a, b := output.Cells[i], output.Cells[j]
		if a.X != b.X {
			return a.X < b.X
		}
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		if a.Colour != b.Colour {
			return a.Colour < b.Colour
		}
		return a.Age < b.Age
	})
	for i, cell := range output.Cells {
		suite.Equal(expectedCells[i], cell)
	}
}

func (suite *APITestSuite) TestMultiGameIsolationAndMultiplayer() {
	gameA := suite.createGameAPI("Arena 128", 128)
	gameB := suite.createGameAPI("Arena 512", 512)

	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	// Connect two shared-board viewers to Game A, and one viewer to Game B.
	a1 := suite.dialGame(ts, gameA.ID)
	defer a1.Close()
	a2 := suite.dialGame(ts, gameA.ID)
	defer a2.Close()
	b1 := suite.dialGame(ts, gameB.ID)
	defer b1.Close()

	for _, c := range []*websocket.Conn{a1, a2, b1} {
		_, _, err := c.ReadMessage()
		suite.Require().NoError(err)
		suite.Require().NoError(ackFrame(c))
	}

	// Verify viewer counts via GET /api/games.
	reqList := httptest.NewRequest("GET", "/api/games", nil)
	wList := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wList, reqList)
	suite.Require().Equal(http.StatusOK, wList.Code)
	var list []games.Info
	suite.Require().NoError(json.Unmarshal(wList.Body.Bytes(), &list))
	suite.Require().Len(list, 2)
	suite.Equal(2, list[0].Viewers)
	suite.Equal(1, list[1].Viewers)

	// Viewer 1 places a cell in Game A -> Viewer 2 receives it; Game B receives nothing.
	sc := &protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 127, Y: 0, Colour: 0x00ff00}},
	}
	suite.Require().NoError(a1.WriteMessage(websocket.BinaryMessage, sc.Encode()))

	for _, c := range []*websocket.Conn{a1, a2} {
		_, msg, err := c.ReadMessage()
		suite.Require().NoError(err)
		suite.Require().NoError(ackFrame(c))
		var out protocol.Output
		suite.Require().NoError(out.Decode(msg))
		suite.Equal(uint32(1), out.CellsCount)
		suite.Equal(uint16(127), out.Cells[0].X)
	}

	// Place a cell at (200, 200) in Game B (distinct from Game A's 128x128 world).
	// If Game A had leaked a frame into Game B, b1's next message would contain
	// (127, 0) instead of only (200, 200).
	scB := &protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 200, Y: 200, Colour: 0x0000ff}},
	}
	suite.Require().NoError(b1.WriteMessage(websocket.BinaryMessage, scB.Encode()))
	_, msgB, err := b1.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(b1))
	var outB protocol.Output
	suite.Require().NoError(outB.Decode(msgB))
	suite.Equal(uint32(1), outB.CellsCount)
	suite.Equal(uint16(200), outB.Cells[0].X)
	suite.Equal(uint16(200), outB.Cells[0].Y)
	suite.Equal(uint32(0x0000ff), outB.Cells[0].Colour)

	// Verify Game A wraps coordinates modulo 128: setting (200, 200) wraps to (72, 72).
	suite.Require().NoError(a1.WriteMessage(websocket.BinaryMessage, scB.Encode()))
	_, msgA, err := a2.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(a2))
	var outA protocol.Output
	suite.Require().NoError(outA.Decode(msgA))
	suite.Equal(uint32(2), outA.CellsCount)
	coordsA := map[[2]uint16]bool{}
	for _, cell := range outA.Cells {
		coordsA[[2]uint16{cell.X, cell.Y}] = true
	}
	suite.True(coordsA[[2]uint16{127, 0}])
	suite.True(coordsA[[2]uint16{72, 72}], "Game A (128x128) must wrap (200,200) to (72,72)")
}

func (suite *APITestSuite) TestRenameAndDeleteClosesWebSocketWith4404() {
	info := suite.createGameAPI("Temp Game", 128)
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	// Rename via PATCH /api/games/:id.
	reqRename := httptest.NewRequest("PATCH", fmt.Sprintf("/api/games/%d", info.ID), strings.NewReader(`{"name":"Renamed Game"}`))
	reqRename.Header.Set("Content-Type", "application/json")
	wRename := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wRename, reqRename)
	suite.Equal(http.StatusOK, wRename.Code)
	suite.Contains(wRename.Body.String(), `"Renamed Game"`)

	c := suite.dialGame(ts, info.ID)
	defer c.Close()
	_, _, err := c.ReadMessage()
	suite.Require().NoError(err)
	suite.Require().NoError(ackFrame(c))

	// Delete via DELETE /api/games/:id -> active WS closes with code 4404.
	reqDel := httptest.NewRequest("DELETE", fmt.Sprintf("/api/games/%d", info.ID), nil)
	wDel := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wDel, reqDel)
	suite.Equal(http.StatusNoContent, wDel.Code)

	suite.Require().NoError(c.SetReadDeadline(time.Now().Add(2 * time.Second)))
	_, _, err = c.ReadMessage()
	suite.Require().Error(err)
	var closeErr *websocket.CloseError
	suite.Require().ErrorAs(err, &closeErr)
	suite.Equal(4404, closeErr.Code)
}

func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}
