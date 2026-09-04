// Package ui provides the interactive pager for rendered HTML.
package ui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hangarbay/sheen/internal/render"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Config carries everything the pager needs to render the document.
type Config struct {
	HTML            string
	Source          string
	Preset          string
	Links           string
	BaseURL         *url.URL
	FetchStylesheet func(string) ([]byte, error)
}

type contentRenderedMsg string

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type model struct {
	cfg    Config
	vp     viewport.Model
	width  int
	height int
	ready  bool
	fatal  error
}

// NewProgram returns a Tea program displaying the document in a scrollable
// viewport.
func NewProgram(cfg Config) *tea.Program {
	m := model{cfg: cfg, vp: viewport.New()}
	return tea.NewProgram(m)
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, renderContent(m.cfg, m.width, m.height-statusBarHeight)

	case contentRenderedMsg:
		m.ready = true
		m.vp.SetWidth(m.width)
		m.vp.SetHeight(m.height - statusBarHeight)
		m.vp.SetContent(string(msg))
		return m, nil

	case errMsg:
		m.fatal = msg.err
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.fatal != nil {
				return m, tea.Quit
			}
		case "g", "home":
			m.vp.GotoTop()
		case "G", "end":
			m.vp.GotoBottom()
		case "u":
			m.vp.HalfPageUp()
		case "d":
			m.vp.HalfPageDown()
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

const statusBarHeight = 1

func (m model) View() tea.View {
	if m.fatal != nil {
		return tea.NewView("sheen: " + m.fatal.Error() + "\n\nPress any key to exit.")
	}
	if !m.ready {
		return tea.NewView("Rendering…")
	}
	var b strings.Builder
	b.WriteString(m.vp.View())
	b.WriteString("\n")
	b.WriteString(m.statusBar())
	return tea.NewView(b.String())
}

func (m model) statusBar() string {
	logo := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0b0b0d")).
		Background(lipgloss.Color("#ff79c6")).
		Bold(true).
		Render(" sheen ")

	pct := fmt.Sprintf(" %3.f%% ", m.vp.ScrollPercent()*100)
	pctStyled := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ff79c6")).
		Render(pct)

	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7d8590"))
	noteWidth := m.width - ansi.StringWidth(logo) - ansi.StringWidth(pctStyled) - 2
	note := ansi.Truncate(" "+m.cfg.Source+" ", max(0, noteWidth), "…")
	noteStyled := noteStyle.Render(note)

	gap := m.width - ansi.StringWidth(logo) - ansi.StringWidth(noteStyled) - ansi.StringWidth(pctStyled)
	if gap < 0 {
		gap = 0
	}
	return logo + noteStyled + strings.Repeat(" ", gap) + pctStyled
}

func renderContent(cfg Config, width, height int) tea.Cmd {
	return func() tea.Msg {
		out, err := render.Render(strings.NewReader(cfg.HTML), render.Options{
			Width:           width,
			Preset:          cfg.Preset,
			BaseURL:         cfg.BaseURL,
			Links:           cfg.Links,
			FetchStylesheet: cfg.FetchStylesheet,
		})
		if err != nil {
			return errMsg{err}
		}
		return contentRenderedMsg(out)
	}
}
