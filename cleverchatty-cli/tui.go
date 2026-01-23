package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

// Messages for Bubble Tea
type chatMsg string
type logMsg string
type spinnerMsg string
type clearSpinnerMsg struct{}
type errorMsg error
type quitMsg struct{}
type initCompleteMsg struct {
	cleverChatty interface{}
	err          error
}

// TUI model
type tuiModel struct {
	chatViewport    viewport.Model
	logsViewport    viewport.Model
	input           textarea.Model
	showLogs        bool
	ready           bool
	initialized     bool
	currentSpinner  string
	width           int
	height          int
	chatContent     *strings.Builder
	logsContent     *strings.Builder
	promptCallback  func(string) error
	cleverChatty    interface{}
}

func newTUIModel(showLogs bool, promptCallback func(string) error) tuiModel {
	input := textarea.New()
	input.Placeholder = "Type your message and press Enter to send (PgUp/PgDn to scroll, /help for commands)"
	input.Focus()
	input.Prompt = "> "
	input.CharLimit = 0
	input.SetHeight(3)
	input.ShowLineNumbers = false
	// Disable the default textarea key handling for Enter
	input.KeyMap.InsertNewline.SetEnabled(false)

	chatContent := &strings.Builder{}
	logsContent := &strings.Builder{}

	// Add initial welcome message
	welcomeStyle := lipgloss.NewStyle().Foreground(tokyoCyan).Bold(true)
	infoStyle := lipgloss.NewStyle().Foreground(tokyoFg)

	chatContent.WriteString(welcomeStyle.Render("Welcome to CleverChatty CLI!"))
	chatContent.WriteString("\n")
	chatContent.WriteString(infoStyle.Render("Type /help for available commands."))
	chatContent.WriteString("\n")

	return tuiModel{
		input:          input,
		showLogs:       showLogs,
		promptCallback: promptCallback,
		chatContent:    chatContent,
		logsContent:    logsContent,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		initCleverChattyCmd,
	)
}

// initCleverChattyCmd is a command that initializes CleverChatty
func initCleverChattyCmd() tea.Msg {
	return initCleverChattyFunc()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
		lpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			// Only allow input if initialized
			if !m.initialized {
				return m, nil
			}
			// Enter submits the prompt
			prompt := strings.TrimSpace(m.input.Value())
			if prompt != "" {
				m.input.Reset()
				// Process the prompt in a goroutine to avoid blocking
				go func() {
					if err := m.promptCallback(prompt); err != nil {
						// Send error message
						program.Send(errorMsg(err))
					}
				}()
			}
			return m, nil
		case tea.KeyPgUp:
			// Scroll chat viewport up
			m.chatViewport.ViewUp()
			return m, nil
		case tea.KeyPgDown:
			// Scroll chat viewport down
			m.chatViewport.ViewDown()
			return m, nil
		case tea.KeyCtrlHome:
			// Ctrl+Home - go to top
			m.chatViewport.GotoTop()
			return m, nil
		case tea.KeyCtrlEnd:
			// Ctrl+End - go to bottom
			m.chatViewport.GotoBottom()
			return m, nil
		case tea.KeyCtrlUp:
			// Ctrl+Up - scroll up one line
			m.chatViewport.LineUp(1)
			return m, nil
		case tea.KeyCtrlDown:
			// Ctrl+Down - scroll down one line
			m.chatViewport.LineDown(1)
			return m, nil
		}

		// Handle Alt+Enter for newline
		if msg.Type == tea.KeyRunes && msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] == '\r' {
			m.input.InsertString("\n")
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate heights: leave space for input (5 lines) + spinner (1 line) + borders
		viewportHeight := msg.Height - 8

		// Calculate widths based on layout
		var chatWidth, logsWidth int
		if m.showLogs {
			// Split view - account for borders (4 chars each side) and gap
			chatWidth = (msg.Width / 2) - 8
			logsWidth = (msg.Width / 2) - 8
		} else {
			chatWidth = msg.Width - 8
		}

		if !m.ready {
			m.chatViewport = viewport.New(chatWidth, viewportHeight)
			m.logsViewport = viewport.New(logsWidth, viewportHeight)
			m.chatViewport.YPosition = 0
			m.logsViewport.YPosition = 0
			// Enable word wrapping
			m.chatViewport.Style = lipgloss.NewStyle().Width(chatWidth)
			m.logsViewport.Style = lipgloss.NewStyle().Width(logsWidth)
			// Set initial content
			m.chatViewport.SetContent(m.chatContent.String())
			m.logsViewport.SetContent(m.logsContent.String())
			// Scroll to bottom
			m.chatViewport.GotoBottom()
			m.logsViewport.GotoBottom()
			m.ready = true
		} else {
			m.chatViewport.Width = chatWidth
			m.logsViewport.Width = logsWidth
			m.chatViewport.Height = viewportHeight
			m.logsViewport.Height = viewportHeight
		}

		// Update input width to match chat viewport
		m.input.SetWidth(chatWidth)

		// Keep scrolled to bottom after resize
		if m.ready {
			m.chatViewport.GotoBottom()
			m.logsViewport.GotoBottom()
		}

	case chatMsg:
		// Wrap text to viewport width if ready
		text := string(msg)
		if m.ready && m.chatViewport.Width > 0 {
			text = wordwrap.String(text, m.chatViewport.Width)
		}
		m.chatContent.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			m.chatContent.WriteString("\n")
		}
		m.chatViewport.SetContent(m.chatContent.String())
		m.chatViewport.GotoBottom()

	case logMsg:
		if m.showLogs {
			// Wrap text to viewport width if ready
			text := string(msg)
			if m.ready && m.logsViewport.Width > 0 {
				text = wordwrap.String(text, m.logsViewport.Width)
			}
			m.logsContent.WriteString(text)
			if !strings.HasSuffix(text, "\n") {
				m.logsContent.WriteString("\n")
			}
			m.logsViewport.SetContent(m.logsContent.String())
			m.logsViewport.GotoBottom()
		}

	case spinnerMsg:
		m.currentSpinner = string(msg)

	case clearSpinnerMsg:
		m.currentSpinner = ""

	case errorMsg:
		errText := errorStyle.Render(fmt.Sprintf("Error: %v\n", msg))
		m.chatContent.WriteString(errText)
		m.chatViewport.SetContent(m.chatContent.String())
		m.chatViewport.GotoBottom()

	case initCompleteMsg:
		if msg.err != nil {
			m.chatContent.WriteString(errorStyle.Render(fmt.Sprintf("Initialization error: %v\n", msg.err)))
			m.chatViewport.SetContent(m.chatContent.String())
		} else {
			m.initialized = true
			m.cleverChatty = msg.cleverChatty
			m.input.Focus()
		}

	case quitMsg:
		return m, tea.Quit
	}

	// Update all components
	m.input, tiCmd = m.input.Update(msg)
	m.chatViewport, vpCmd = m.chatViewport.Update(msg)
	if m.showLogs {
		m.logsViewport, lpCmd = m.logsViewport.Update(msg)
	}

	return m, tea.Batch(tiCmd, vpCmd, lpCmd)
}

func (m tuiModel) View() string {
	if !m.ready {
		return "Initializing UI..."
	}

	// Styles with titles
	chatStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tokyoBlue).
		Padding(1, 2)

	logsStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tokyoGreen).
		Padding(1, 2)

	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tokyoPurple).
		Padding(0, 1)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tokyoCyan)

	scrollIndicatorStyle := lipgloss.NewStyle().
		Foreground(tokyoGreen).
		Italic(true)

	// Build the view
	var content string

	// Check if we can scroll up (not at the top)
	scrollIndicator := ""
	if m.chatViewport.YOffset > 0 {
		scrollIndicator = scrollIndicatorStyle.Render("↑ More messages above (PgUp/PgDn to scroll, Ctrl+Home for top) ↑") + "\n"
	}

	if m.showLogs {
		// Split view with titles
		chatTitle := titleStyle.Render("Chat")
		logsTitle := titleStyle.Render("Logs")

		chatContent := chatTitle + "\n" + scrollIndicator + m.chatViewport.View()
		logsContent := logsTitle + "\n" + m.logsViewport.View()

		chatView := chatStyle.
			Width(m.chatViewport.Width + 4).
			Height(m.chatViewport.Height + 2).
			Render(chatContent)

		logsView := logsStyle.
			Width(m.logsViewport.Width + 4).
			Height(m.logsViewport.Height + 2).
			Render(logsContent)

		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			chatView,
			logsView,
		)
	} else {
		// Single chat view
		chatTitle := titleStyle.Render("Chat")
		chatContent := chatTitle + "\n" + scrollIndicator + m.chatViewport.View()
		content = chatStyle.
			Width(m.chatViewport.Width + 4).
			Height(m.chatViewport.Height + 2).
			Render(chatContent)
	}

	// Add spinner if active or show initialization message
	spinnerView := ""
	if !m.initialized {
		spinnerView = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			Render("🔄 Connecting to services...") + "\n"
	} else if m.currentSpinner != "" {
		spinnerView = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			Render(m.currentSpinner) + "\n"
	}

	// Input area
	inputWidth := m.chatViewport.Width
	if m.showLogs {
		inputWidth = m.width - 4
	}

	inputView := inputStyle.
		Width(inputWidth).
		Render(m.input.View())

	// Combine everything
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		spinnerView,
		inputView,
	)
}

// Global program instance to send messages
var program *tea.Program

// tuiLogWriter is a custom writer to capture logs and send them to the TUI
type tuiLogWriter struct{}

func (lw *tuiLogWriter) Write(p []byte) (n int, err error) {
	if program != nil {
		program.Send(logMsg(string(p)))
	}
	return len(p), nil
}

// Helper functions to send messages to the TUI
func tuiSendChat(msg string) {
	if program != nil {
		program.Send(chatMsg(msg))
	}
}

func tuiSendSpinner(msg string) {
	if program != nil {
		program.Send(spinnerMsg(msg))
	}
}

func tuiClearSpinner() {
	if program != nil {
		program.Send(clearSpinnerMsg{})
	}
}

func tuiSendError(err error) {
	if program != nil {
		program.Send(errorMsg(err))
	}
}

func tuiQuit() {
	if program != nil {
		program.Send(quitMsg{})
	}
}
