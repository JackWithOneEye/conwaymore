package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/JackWithOneEye/conwaymore/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	host := flag.String("host", "localhost:8080", "API server host:port")
	gameID := flag.Int64("game", 0, "Game ID to connect to (defaults to first available game)")
	flag.Parse()

	if len(os.Getenv("DEBUG")) > 0 {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
	}
	p := tea.NewProgram(&tui.UIModel{APIHost: *host, GameID: *gameID}, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Printf("Error running terminal UI: %v", err)
		os.Exit(1)
	}
}
