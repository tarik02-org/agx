// Package launcher is the full-screen picker bare agx opens on a terminal: one search field over
// harnesses to start and sessions to resume.
package launcher

import (
	"cmp"
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/sessions"
)

type Harness struct{ Name, Builtin string }

type Options struct {
	// Harnesses are offered for new sessions in this order.
	Harnesses []Harness
	// Sessions are every listed session, newest first.
	Sessions []sessions.Entry
	// Directory is the canonical current directory, which the here scope covers.
	Directory string
	Profile   colorprofile.Profile
	// Notes are shown under the key help, such as stores that could not be read.
	Notes []string
}

type Action int

const (
	Quit Action = iota
	Interrupt
	Start
	Resume
)

// Choice is what the user picked. Harness names the harness to start, or to resume Session with.
type Choice struct {
	Action  Action
	Harness string
	Session sessions.Entry
}

// Run shows the launcher until the user picks or quits. The terminal is restored when it returns,
// so the caller can hand it to a harness.
func Run(ctx context.Context, opts Options) (Choice, error) {
	final, err := tea.NewProgram(
		newModel(opts),
		tea.WithContext(ctx),
		tea.WithColorProfile(opts.Profile),
	).Run()
	if err != nil {
		return Choice{}, err
	}
	return final.(model).choice, nil
}

type row struct {
	harness *Harness
	entry   *sessions.Entry
}

type model struct {
	opts     Options
	here     []sessions.Entry
	input    textinput.Model
	all      bool
	rows     []row
	cursor   int
	offset   int
	width    int
	height   int
	picker   []string
	picked   int
	choice   Choice
	styles   styles
	now      time.Time
	plain    bool
	quitting bool
}

type styles struct {
	heading, selected, muted, harness, path, warn lipgloss.Style
}

func newModel(opts Options) model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Search harnesses and sessions"
	// The terminal's own cursor stays visible when output is unstyled.
	input.SetVirtualCursor(false)
	s := textinput.DefaultStyles(true)
	s.Cursor.Blink = false
	input.SetStyles(s)
	input.Focus()
	m := model{
		opts:   opts,
		here:   sessions.Within(opts.Sessions, opts.Directory),
		input:  input,
		width:  80,
		height: 24,
		now:    time.Now(),
		plain:  opts.Profile <= colorprofile.ASCII,
		styles: styles{
			heading:  lipgloss.NewStyle().Bold(true),
			selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan),
			muted:    lipgloss.NewStyle().Faint(true),
			harness:  lipgloss.NewStyle().Foreground(lipgloss.Cyan),
			path:     lipgloss.NewStyle().Foreground(lipgloss.Blue),
			warn:     lipgloss.NewStyle().Foreground(lipgloss.Yellow),
		},
	}
	m.input.SetWidth(m.width - len(m.input.Prompt) - 1)
	m.refresh()
	return m
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(m.width-len(m.input.Prompt)-1, 1))
		m.scroll()
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.PasteMsg:
		var cmd tea.Cmd
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.cursor = 0
			m.refresh()
		}
		return m, cmd
	}
	return m, nil
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.picker != nil {
		switch msg.String() {
		case "ctrl+c":
			return m.quit(Choice{Action: Interrupt})
		case "esc", "ctrl+h":
			m.picker = nil
		case "up", "ctrl+p":
			m.picked = max(m.picked-1, 0)
		case "down", "ctrl+n":
			m.picked = min(m.picked+1, len(m.picker)-1)
		case "enter":
			return m.quit(Choice{Action: Resume, Harness: m.picker[m.picked], Session: *m.rows[m.cursor].entry})
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		return m.quit(Choice{Action: Interrupt})
	case "esc":
		return m.quit(Choice{Action: Quit})
	case "up", "ctrl+p":
		m.cursor = max(m.cursor-1, 0)
	case "down", "ctrl+n":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
	case "pgup":
		m.cursor = max(m.cursor-m.visible(), 0)
	case "pgdown":
		m.cursor = min(m.cursor+m.visible(), max(len(m.rows)-1, 0))
	case "tab":
		m.all = !m.all
		m.refresh()
		m.cursor = 0
		for i, r := range m.rows {
			if r.entry != nil {
				m.cursor = i
				break
			}
		}
	case "ctrl+h":
		if len(m.rows) > 0 && m.rows[m.cursor].entry != nil {
			e := m.rows[m.cursor].entry
			m.picker = []string{}
			m.picked = 0
			for _, h := range m.opts.Harnesses {
				if h.Builtin == e.Builtin {
					if h.Name == e.Harness {
						m.picked = len(m.picker)
					}
					m.picker = append(m.picker, h.Name)
				}
			}
		}
	case "enter":
		if len(m.rows) == 0 {
			return m, nil
		}
		r := m.rows[m.cursor]
		if r.harness != nil {
			return m.quit(Choice{Action: Start, Harness: r.harness.Name})
		}
		return m.quit(Choice{Action: Resume, Harness: r.entry.Harness, Session: *r.entry})
	default:
		var cmd tea.Cmd
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.cursor = 0
			m.refresh()
		}
		m.scroll()
		return m, cmd
	}
	m.scroll()
	return m, nil
}

func (m model) quit(choice Choice) (tea.Model, tea.Cmd) {
	m.choice = choice
	m.quitting = true
	return m, tea.Quit
}

// refresh recomputes the rows for the query and scope.
func (m *model) refresh() {
	query := strings.TrimSpace(m.input.Value())
	m.rows = m.rows[:0]
	names := make([]string, len(m.opts.Harnesses))
	for i, h := range m.opts.Harnesses {
		names[i] = h.Name
	}
	if query == "" {
		for i := range m.opts.Harnesses {
			m.rows = append(m.rows, row{harness: &m.opts.Harnesses[i]})
		}
	} else {
		for _, match := range fuzzy.Find(strings.ReplaceAll(query, " ", ""), names) {
			m.rows = append(m.rows, row{harness: &m.opts.Harnesses[match.Index]})
		}
	}
	scope := m.here
	if m.all {
		scope = m.opts.Sessions
	}
	found := sessions.Search(scope, query)
	for i := range found {
		m.rows = append(m.rows, row{entry: &found[i]})
	}
	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	m.scroll()
}

// Lines above and below the list: the search field and scope, and the key help with notes.
func (m model) chrome() int { return 3 + 2 + len(m.opts.Notes) }

func (m model) visible() int { return max(m.height-m.chrome(), 3) }

// scroll keeps the selected row within the visible part of the list.
func (m *model) scroll() {
	line := m.lineOf(m.cursor)
	visible := m.visible()
	if line < m.offset {
		// Show a group's heading together with its first row.
		m.offset = max(line-1, 0)
	}
	if line >= m.offset+visible {
		m.offset = line - visible + 1
	}
	m.offset = max(min(m.offset, len(m.body())-visible), 0)
}

// lineOf is the body line holding row i.
func (m model) lineOf(i int) int {
	_, lines := m.layout()
	if i < len(lines) {
		return lines[i]
	}
	return 0
}

func (m model) body() []string {
	body, _ := m.layout()
	return body
}

// layout renders the list body and records which line holds each row.
func (m model) layout() ([]string, []int) {
	body := []string{}
	lines := make([]int, len(m.rows))
	harnesses, entries := 0, 0
	for _, r := range m.rows {
		if r.harness != nil {
			harnesses++
		} else {
			entries++
		}
	}
	query := strings.TrimSpace(m.input.Value())
	if harnesses > 0 {
		body = append(body, m.render(m.styles.heading, "New session"))
	}
	width := 0
	for _, r := range m.rows {
		if r.entry != nil {
			width = max(width, ansi.StringWidth(r.entry.Harness))
		} else {
			width = max(width, ansi.StringWidth(r.harness.Name))
		}
	}
	width = min(width, 20)
	for i, r := range m.rows {
		if r.harness == nil {
			if i == harnesses {
				if harnesses > 0 {
					body = append(body, "")
				}
				body = append(body, m.sessionsHeading(entries))
			}
			lines[i] = len(body)
			body = append(body, m.sessionRow(*r.entry, width, i == m.cursor))
			continue
		}
		lines[i] = len(body)
		text := pad(r.harness.Name, width)
		if r.harness.Builtin != r.harness.Name {
			text += "  " + m.render(m.styles.muted, r.harness.Builtin)
		}
		body = append(body, m.line(text, i == m.cursor))
	}
	if entries == 0 {
		if harnesses > 0 {
			body = append(body, "")
		}
		body = append(body, m.sessionsHeading(0))
		switch {
		case query != "":
			body = append(body, "  "+m.render(m.styles.muted, "No matching sessions."))
		case !m.all:
			body = append(body, "  "+m.render(m.styles.muted, "No sessions in this directory. Tab shows all."))
		default:
			body = append(body, "  "+m.render(m.styles.muted, "No sessions yet."))
		}
	}
	return body, lines
}

func (m model) sessionsHeading(count int) string {
	scope := "here"
	if m.all {
		scope = "all directories"
	}
	return m.render(m.styles.heading, "Sessions") + m.render(m.styles.muted, " · "+scope+" · "+strconv.Itoa(count))
}

func (m model) sessionRow(e sessions.Entry, harnessWidth int, selected bool) string {
	ago := pad(ui.Ago(e.Updated, m.now), 3)
	room := m.width - 4 - harnessWidth - 2 - len(ago) - 2
	dir := ""
	if e.Directory != m.opts.Directory && m.width >= 60 {
		dir = ui.ShortPath(e.Directory, m.opts.Directory)
		if limit := room * 2 / 5; ansi.StringWidth(dir) > limit {
			dir = ansi.TruncateLeft(dir, ansi.StringWidth(dir)-limit+1, "…")
		}
		room -= ansi.StringWidth(dir) + 2
	}
	text := e.Title
	style := lipgloss.NewStyle()
	if text == "" {
		text = e.Message
	}
	if text == "" {
		text = "(no message)"
		style = m.styles.muted
	}
	text = ansi.Truncate(text, max(room, 8), "…")
	line := m.render(m.styles.harness, pad(e.Harness, harnessWidth)) + "  " + m.render(m.styles.muted, ago) + "  " +
		m.render(style, pad(text, max(room, 0)))
	if dir != "" {
		line += "  " + m.render(m.styles.path, dir)
	}
	return m.line(line, selected)
}

// line marks the selected row; the marker carries the selection when output is unstyled.
func (m model) line(text string, selected bool) string {
	if !selected {
		return "  " + text
	}
	if m.plain {
		return "> " + text
	}
	return m.styles.selected.Render("›") + " " + text
}

func (m model) render(style lipgloss.Style, text string) string {
	if m.plain || text == "" {
		return text
	}
	return style.Render(text)
}

func (m model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(m.input.View() + "\n")
	scope := "here: " + ui.ShortPath(m.opts.Directory, "")
	if m.all {
		scope = "all directories"
	}
	b.WriteString(m.render(m.styles.muted, scope) + "\n\n")
	visible := m.visible()
	if m.picker != nil {
		e := m.rows[m.cursor].entry
		title := cmp.Or(e.Title, e.Message, e.ID)
		b.WriteString(m.render(m.styles.heading, "Resume with") + " " +
			m.render(m.styles.muted, ansi.Truncate(title, max(m.width-14, 8), "…")) + "\n")
		for i, name := range m.picker {
			b.WriteString(m.line(name, i == m.picked) + "\n")
		}
		for range max(visible-len(m.picker)-1, 0) {
			b.WriteString("\n")
		}
	} else {
		body := m.body()
		end := min(m.offset+visible, len(body))
		for _, line := range body[m.offset:end] {
			b.WriteString(line + "\n")
		}
		for range visible - (end - m.offset) {
			b.WriteString("\n")
		}
	}
	help := "↑↓ move · enter open · tab here/all · ctrl-h harness · esc quit"
	if m.picker != nil {
		help = "↑↓ move · enter resume · esc back"
	}
	b.WriteString("\n" + m.render(m.styles.muted, help))
	for _, note := range m.opts.Notes {
		b.WriteString("\n" + m.render(m.styles.warn, note))
	}
	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	if c := m.input.Cursor(); c != nil && m.picker == nil {
		v.Cursor = c
	}
	return v
}

func pad(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}
