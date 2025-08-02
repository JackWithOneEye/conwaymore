package main

import (
	"fmt"
	"log"
	"os"

	"github.com/JackWithOneEye/conwaymore/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Getenv("DEBUG")) > 0 {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			fmt.Println("fatal:", err)
			os.Exit(1)
		}
		defer f.Close()
	}
	p := tea.NewProgram(&tui.UIModel{}, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		log.Printf("Error running terminal UI: %v", err)
		os.Exit(1)
	}
}
