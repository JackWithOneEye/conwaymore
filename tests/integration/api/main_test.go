package api_test

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"net/url"

	"os"

	"context"

	"github.com/JackWithOneEye/conwaymore/internal/database"
	"github.com/JackWithOneEye/conwaymore/internal/engine"
	"github.com/JackWithOneEye/conwaymore/internal/protocol"
	"github.com/JackWithOneEye/conwaymore/internal/server"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
)

type APITestSuite struct {
	suite.Suite
	server *http.Server
	db     database.DatabaseService
	dbFile string
	ctx    context.Context
	cancel context.CancelFunc
}

// testDatabaseConfig implements DatabaseConfig for integration tests
type testDatabaseConfig struct {
	dbUrl string
}

func (c *testDatabaseConfig) DBUrl() string { return c.dbUrl }

func (suite *APITestSuite) SetupTest() {
	// Create a unique temporary database file for each test
	tmpFile, err := os.CreateTemp("", "test_integration_*.db")
	suite.Require().NoError(err)
	dbFile := tmpFile.Name()
	tmpFile.Close()

	cfg := &testConfig{port: 8080, worldSize: 1024}
	dbCfg := &testDatabaseConfig{dbUrl: dbFile}
	db := database.NewDatabaseService(dbCfg)
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.NewEngine(cfg, nil, ctx)
	suite.server = server.NewServer(cfg, db, eng, ctx)
	suite.db = db
	suite.dbFile = dbFile
	suite.ctx = ctx
	suite.cancel = cancel
}

func (suite *APITestSuite) TearDownTest() {
	// Cleanup after each test
	if suite.cancel != nil {
		suite.cancel()
	}
	suite.server.Close()
	suite.db.Close()
	if suite.dbFile != "" {
		os.Remove(suite.dbFile)
	}
}

func (suite *APITestSuite) TestHealthCheck() {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)
}

func (suite *APITestSuite) TestGameState() {
	// Test game state endpoints
	suite.T().Run("get game state", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/game", nil)
		w := httptest.NewRecorder()
		suite.server.Handler.ServeHTTP(w, req)

		suite.Equal(http.StatusOK, w.Code)

		// Verify the response contains the expected game state information
		body := w.Body.String()
		suite.Contains(body, "value=\"30\"")        // Cell size from test config
		suite.Contains(body, "value=\"#ffffff\"")   // Default cell color
		suite.Contains(body, "PLAY")                // Initial state is paused
		suite.Contains(body, "value=\"81.000000\"") // Speed slider value
		suite.Contains(body, "100 ms")              // Speed label
	})
}

func (suite *APITestSuite) TestSaveEndpoint() {
	// Simulate a last output by calling /game to trigger output
	reqGame := httptest.NewRequest("GET", "/game", nil)
	wGame := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(wGame, reqGame)

	// Now call /save
	req := httptest.NewRequest("POST", "/save", nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)

	// Check that the response is successful
	suite.Equal(http.StatusOK, w.Code)

	// Check that the seed was written
	seed, err := suite.db.GetSeed()
	suite.NoError(err)
	suite.NotEmpty(seed)
}

func (suite *APITestSuite) TestPlayWebSocket() {
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	suite.NoError(err)
	u.Scheme = "ws"
	u.Path = "/play"

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	suite.NoError(err)
	defer c.Close()

	// Expect to receive a message (the initial game state)
	_, msg, err := c.ReadMessage()
	suite.NoError(err)
	suite.NotEmpty(msg)
}

func (suite *APITestSuite) TestPlayAndSaveGameState() {
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	suite.NoError(err)
	u.Scheme = "ws"
	u.Path = "/play"

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	suite.NoError(err)
	defer c.Close()

	// Send a command to add a cell at (0,0) with color #ff0000 using the binary protocol
	setCells := protocol.SetCells{
		Count: 1,
		Cells: []protocol.Cell{{X: 0, Y: 0, Colour: 0xff0000, Age: 0}},
	}
	cmd := setCells.Encode()
	err = c.WriteMessage(websocket.BinaryMessage, cmd)
	suite.NoError(err)

	// Wait for the engine to process the SetCells command
	_, msg, err := c.ReadMessage()
	suite.NoError(err)
	var decodedOutput protocol.Output
	err = decodedOutput.Decode(msg)
	suite.NoError(err)
	suite.T().Logf("After SetCells, decoded output: %+v", decodedOutput)

	// Call /save to save the game state
	req := httptest.NewRequest("POST", "/save", nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)

	// Check that the seed was written and contains the expected cell
	seed, err := suite.db.GetSeed()
	suite.NoError(err)
	suite.NotEmpty(seed)

	// Decode the seed and verify it contains the cell at (0,0) with color #ff0000
	var output protocol.Output
	err = output.Decode(seed)
	suite.NoError(err)
	suite.Equal(uint32(1), output.CellsCount)
	suite.NotEmpty(output.Cells, "Cells slice should not be empty")
	suite.Equal(uint16(0), output.Cells[0].X)
	suite.Equal(uint16(0), output.Cells[0].Y)
	suite.Equal(uint32(0xff0000), output.Cells[0].Colour)
}

func (suite *APITestSuite) TestGliderPattern() {
	ts := httptest.NewServer(suite.server.Handler)
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	suite.NoError(err)
	u.Scheme = "ws"
	u.Path = "/play"

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	suite.NoError(err)
	defer c.Close()

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
	cmd := setCells.Encode()
	err = c.WriteMessage(websocket.BinaryMessage, cmd)
	suite.NoError(err)

	_, msg, err := c.ReadMessage()
	suite.NoError(err)
	var decodedOutput protocol.Output
	err = decodedOutput.Decode(msg)
	suite.NoError(err)

	for i := 0; i < 5; i++ {
		nextCmd := protocol.Command{Cmd: protocol.Next}
		err = c.WriteMessage(websocket.BinaryMessage, nextCmd.Encode())
		suite.NoError(err)
		_, msg, err = c.ReadMessage()
		suite.NoError(err)
		err = decodedOutput.Decode(msg)
		suite.NoError(err)
	}

	req := httptest.NewRequest("POST", "/save", nil)
	w := httptest.NewRecorder()
	suite.server.Handler.ServeHTTP(w, req)
	suite.Equal(http.StatusOK, w.Code)

	seed, err := suite.db.GetSeed()
	suite.NoError(err)
	suite.NotEmpty(seed)

	var output protocol.Output
	err = output.Decode(seed)
	suite.NoError(err)
	suite.NotEmpty(output.Cells, "Cells slice should not be empty")

	suite.Equal(uint32(5), output.CellsCount)

	expectedCells := []protocol.Cell{
		{X: 11, Y: 13, Colour: 0x00ff00, Age: 3},
		{X: 12, Y: 11, Colour: 0x00ff00, Age: 0},
		{X: 12, Y: 13, Colour: 0x00ff00, Age: 2},
		{X: 13, Y: 12, Colour: 0x00ff00, Age: 1},
		{X: 13, Y: 13, Colour: 0x00ff00, Age: 0},
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
		suite.Equal(expectedCells[i].X, cell.X, "Cell X coordinate mismatch")
		suite.Equal(expectedCells[i].Y, cell.Y, "Cell Y coordinate mismatch")
		suite.Equal(expectedCells[i].Colour, cell.Colour, "Cell color mismatch")
		suite.Equal(expectedCells[i].Age, cell.Age, "Cell age mismatch")
	}
}

func TestAPISuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

type testConfig struct {
	port      uint
	worldSize uint
}

func (c *testConfig) Port() uint      { return c.port }
func (c *testConfig) WorldSize() uint { return c.worldSize }
