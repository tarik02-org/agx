// Package prompt asks a person at a terminal for missing values: a line of text, a yes/no
// confirmation, or a multiple selection. Prompts draw inline on stderr with Bubble Tea, in the
// launcher's styling, and leave the answer on screen.
package prompt

import (
	"context"
	"errors"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/tarik02-org/agx/internal/cli/ui"
)

var (
	// ErrInterrupted is Ctrl-C; callers exit with 130.
	ErrInterrupted = errors.New("interrupted")
	// ErrCancelled is Esc.
	ErrCancelled = errors.New("cancelled")
)

type styles struct {
	question, answer, selected, muted, problem lipgloss.Style
	plain                                      bool
}

func newStyles(profile colorprofile.Profile) styles {
	return styles{
		question: lipgloss.NewStyle().Bold(true),
		answer:   lipgloss.NewStyle().Foreground(lipgloss.Cyan),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan),
		muted:    lipgloss.NewStyle().Faint(true),
		problem:  lipgloss.NewStyle().Foreground(lipgloss.Red),
		plain:    profile <= colorprofile.ASCII,
	}
}

func (s styles) render(style lipgloss.Style, text string) string {
	if s.plain || text == "" {
		return text
	}
	return style.Render(text)
}

// outcome ends a prompt: done with an answer, or one of the errors.
type outcome struct {
	done bool
	err  error
}

func run[M tea.Model](ctx context.Context, model M) (M, error) {
	final, err := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithColorProfile(ui.Stderr.Profile()),
	).Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return model, ErrInterrupted
		}
		return model, err
	}
	return final.(M), nil
}

func ended(key string) (outcome, bool) {
	switch key {
	case "ctrl+c":
		return outcome{err: ErrInterrupted}, true
	case "esc":
		return outcome{err: ErrCancelled}, true
	}
	return outcome{}, false
}

type inputModel struct {
	label    string
	input    textinput.Model
	validate func(string) error
	problem  string
	outcome  outcome
	styles   styles
}

// Input asks for a line of text; validate, when set, rejects answers until it returns nil.
func Input(ctx context.Context, label, placeholder string, validate func(string) error) (string, error) {
	s := newStyles(ui.Stderr.Profile())
	input := textinput.New()
	input.Prompt = s.render(s.question, label) + " "
	input.Placeholder = placeholder
	input.SetVirtualCursor(false)
	inputStyles := textinput.DefaultStyles(true)
	inputStyles.Cursor.Blink = false
	input.SetStyles(inputStyles)
	input.Focus()
	m, err := run(ctx, inputModel{label: label, input: input, validate: validate, styles: s})
	if err != nil {
		return "", err
	}
	if m.outcome.err != nil {
		return "", m.outcome.err
	}
	return strings.TrimSpace(m.input.Value()), nil
}

func (m inputModel) Init() tea.Cmd { return nil }

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.input.SetWidth(max(msg.Width-ansi.StringWidth(m.input.Prompt)-1, 1))
		return m, nil
	case tea.KeyPressMsg:
		if o, ok := ended(msg.String()); ok {
			m.outcome = o
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			value := strings.TrimSpace(m.input.Value())
			if m.validate != nil {
				if err := m.validate(value); err != nil {
					m.problem = err.Error()
					return m, nil
				}
			}
			m.outcome = outcome{done: true}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.problem = ""
	return m, cmd
}

func (m inputModel) View() tea.View {
	if m.outcome.done || m.outcome.err != nil {
		return tea.NewView(m.styles.render(m.styles.question, m.label) + " " +
			m.styles.render(m.styles.answer, strings.TrimSpace(m.input.Value())) + "\n")
	}
	content := m.input.View()
	if m.problem != "" {
		content += "\n" + m.styles.render(m.styles.problem, m.problem)
	}
	v := tea.NewView(content)
	v.Cursor = m.input.Cursor()
	return v
}

type confirmModel struct {
	question string
	yes      bool
	outcome  outcome
	styles   styles
}

// Confirm asks a yes/no question; Enter takes the default.
func Confirm(ctx context.Context, question string, yes bool) (bool, error) {
	m, err := run(ctx, confirmModel{question: question, yes: yes, styles: newStyles(ui.Stderr.Profile())})
	if err != nil {
		return false, err
	}
	return m.yes, m.outcome.err
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if o, ok := ended(key.String()); ok {
		m.outcome = o
		return m, tea.Quit
	}
	switch strings.ToLower(key.String()) {
	case "y":
		m.yes = true
	case "n":
		m.yes = false
	case "enter":
	default:
		return m, nil
	}
	m.outcome = outcome{done: true}
	return m, tea.Quit
}

func (m confirmModel) View() tea.View {
	hint := "[Y/n]"
	if !m.yes {
		hint = "[y/N]"
	}
	line := m.styles.render(m.styles.question, m.question) + " " + m.styles.render(m.styles.muted, hint) + " "
	switch {
	case m.outcome.done && m.yes:
		line += m.styles.render(m.styles.answer, "yes") + "\n"
	case m.outcome.done:
		line += m.styles.render(m.styles.answer, "no") + "\n"
	case m.outcome.err != nil:
		line += "\n"
	}
	return tea.NewView(line)
}

// Item is one choice of a selection.
type Item struct {
	Label, Detail string
	Selected      bool
}

type selectModel struct {
	title   string
	items   []Item
	cursor  int
	width   int
	problem string
	outcome outcome
	styles  styles
}

// Select asks for any number of items; Space toggles one, a toggles all, and Enter accepts a
// selection of at least one. It returns the chosen indexes in order.
func Select(ctx context.Context, title string, items []Item) ([]int, error) {
	m, err := run(ctx, selectModel{title: title, items: items, width: 80, styles: newStyles(ui.Stderr.Profile())})
	if err != nil {
		return nil, err
	}
	if m.outcome.err != nil {
		return nil, m.outcome.err
	}
	chosen := []int{}
	for i, item := range m.items {
		if item.Selected {
			chosen = append(chosen, i)
		}
	}
	return chosen, nil
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		if o, ok := ended(msg.String()); ok {
			m.outcome = o
			return m, tea.Quit
		}
		m.problem = ""
		switch msg.String() {
		case "up", "k", "ctrl+p":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j", "ctrl+n":
			m.cursor = min(m.cursor+1, len(m.items)-1)
		case "space", " ", "x":
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		case "a":
			all := true
			for _, item := range m.items {
				all = all && item.Selected
			}
			for i := range m.items {
				m.items[i].Selected = !all
			}
		case "enter":
			for _, item := range m.items {
				if item.Selected {
					m.outcome = outcome{done: true}
					return m, tea.Quit
				}
			}
			m.problem = "Select at least one with Space."
		}
	}
	return m, nil
}

func (m selectModel) View() tea.View {
	s := m.styles
	if m.outcome.done {
		chosen := []string{}
		for _, item := range m.items {
			if item.Selected {
				chosen = append(chosen, item.Label)
			}
		}
		return tea.NewView(s.render(s.question, m.title) + " " + s.render(s.answer, strings.Join(chosen, ", ")) + "\n")
	}
	if m.outcome.err != nil {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(s.render(s.question, m.title) + "\n")
	width := 0
	for _, item := range m.items {
		width = max(width, ansi.StringWidth(item.Label))
	}
	for i, item := range m.items {
		mark := "[ ]"
		if item.Selected {
			mark = "[x]"
		}
		pointer := "  "
		if i == m.cursor {
			pointer = "> "
			if !s.plain {
				pointer = s.render(s.selected, "›") + " "
			}
		}
		label := item.Label + strings.Repeat(" ", width-ansi.StringWidth(item.Label))
		line := pointer + mark + " " + label
		if i == m.cursor && !s.plain {
			line = pointer + s.render(s.selected, mark+" "+label)
		}
		if item.Detail != "" {
			line += "  " + s.render(s.muted, item.Detail)
		}
		b.WriteString(ansi.Truncate(line, max(m.width, 20), "…") + "\n")
	}
	help := "↑↓ move · space toggle · a all · enter accept · esc cancel"
	if m.problem != "" {
		help = s.render(s.problem, m.problem)
	} else {
		help = s.render(s.muted, help)
	}
	b.WriteString(help + "\n")
	return tea.NewView(b.String())
}
