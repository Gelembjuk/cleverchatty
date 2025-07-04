package main

import (
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	renderer *glamour.TermRenderer
)

var (
	// Tokyo Night theme colors
	tokyoPurple = lipgloss.Color("99")  // #9d7cd8
	tokyoCyan   = lipgloss.Color("73")  // #7dcfff
	tokyoBlue   = lipgloss.Color("111") // #7aa2f7
	tokyoGreen  = lipgloss.Color("120") // #73daca
	tokyoRed    = lipgloss.Color("203") // #f7768e
	//tokyoOrange = lipgloss.Color("215") // #ff9e64
	tokyoFg = lipgloss.Color("189") // #c0caf5
	//tokyoGray   = lipgloss.Color("237") // #3b4261
	tokyoBg = lipgloss.Color("234") // #1a1b26

	promptStyle = lipgloss.NewStyle().
			Foreground(tokyoBlue)

	responseStyle = lipgloss.NewStyle().
			Foreground(tokyoFg)

	errorStyle = lipgloss.NewStyle().
			Foreground(tokyoRed).
			PaddingLeft(2).
			Bold(true)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			PaddingLeft(2).
			Bold(true)

	//descriptionStyle = lipgloss.NewStyle().
	//			Foreground(tokyoFg).
	//			PaddingLeft(2).
	//			PaddingBottom(1)

	contentStyle = lipgloss.NewStyle().
			Background(tokyoBg).
			PaddingLeft(4).
			PaddingRight(4)
)

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Fallback width
	}
	return width - 20
}

func updateRenderer() error {
	width := getTerminalWidth()
	var err error
	renderer, err = glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.TokyoNightStyle),
		glamour.WithWordWrap(width),
	)
	return err
}

func releaseActionSpinner() {
	if actionInProgress {
		actionInProgress = false
		actionChannel <- true
		<-actionCanceledChannel
	}
}

func showSpinner(text string) {
	if actionInProgress {
		releaseActionSpinner()
	}
	actionInProgress = true
	go func() {
		_ = spinner.New().Title(text).Action(func() {
			<-actionChannel
		}).Run()

		actionCanceledChannel <- true
	}()
}
