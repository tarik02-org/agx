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
	"github.com/dustin/go-humanize"
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
	Notes        []string
	LoadSessions func(context.Context, func(SessionUpdate) error) error
}

type SessionUpdate struct {
	Entries []sessions.Entry
	Notes   []string
	Loading bool
}

type sessionResult struct {
	update SessionUpdate
	err    error
	closed bool
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newModel(opts)
	if opts.LoadSessions != nil {
		updates := make(chan sessionResult)
		m.updates = updates
		go func() {
			defer close(updates)
			err := opts.LoadSessions(ctx, func(update SessionUpdate) error {
				select {
				case updates <- sessionResult{update: update}:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			if err != nil {
				select {
				case updates <- sessionResult{err: err}:
				case <-ctx.Done():
				}
			}
		}()
	}
	final, err := tea.NewProgram(
		m,
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

type lineKind int

const (
	listRow lineKind = iota
	harnessHeading
	sessionHeading
	blankLine
	emptySessions
)

type bodyLine struct {
	kind lineKind
	row  int
}

type model struct {
	opts          Options
	here          []sessions.Entry
	input         textinput.Model
	all           bool
	rows          []row
	lines         []bodyLine
	rowLines      []int
	harnessWidth  int
	sessionCount  int
	cursor        int
	offset        int
	width         int
	height        int
	picker        []string
	picked        int
	pickerSession sessions.Entry
	updates       <-chan sessionResult
	loading       bool
	notes         []string
	choice        Choice
	styles        styles
	now           time.Time
	plain         bool
	quitting      bool
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
		opts:    opts,
		here:    sessions.Within(opts.Sessions, opts.Directory),
		input:   input,
		width:   80,
		height:  24,
		now:     time.Now(),
		plain:   opts.Profile <= colorprofile.ASCII,
		loading: opts.LoadSessions != nil,
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

func (m model) Init() tea.Cmd { return m.nextSessions() }

func (m model) nextSessions() tea.Cmd {
	if m.updates == nil {
		return nil
	}
	return func() tea.Msg {
		result, ok := <-m.updates
		if !ok {
			return sessionResult{closed: true}
		}
		return result
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionResult:
		if msg.closed {
			m.loading = false
			return m, nil
		}
		if msg.err != nil {
			m.loading = false
			m.notes = append(m.notes, "Cannot list sessions: "+msg.err.Error())
			m.scroll()
			return m, m.nextSessions()
		}
		var selected row
		if m.cursor < len(m.rows) {
			selected = m.rows[m.cursor]
		}
		m.opts.Sessions = msg.update.Entries
		m.now = time.Now()
		m.here = sessions.Within(msg.update.Entries, m.opts.Directory)
		m.notes, m.loading = msg.update.Notes, msg.update.Loading
		m.refresh()
		for i, candidate := range m.rows {
			if selected.harness != nil && candidate.harness != nil && selected.harness.Name == candidate.harness.Name ||
				selected.entry != nil && candidate.entry != nil && selected.entry.Builtin == candidate.entry.Builtin &&
					selected.entry.ID == candidate.entry.ID {
				m.cursor = i
				break
			}
		}
		m.scroll()
		return m, m.nextSessions()
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
			return m.quit(Choice{Action: Resume, Harness: m.picker[m.picked], Session: m.pickerSession})
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
			m.pickerSession = *e
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
	m.layoutRows()
	m.scroll()
}

// Lines above and below the list: the search field and scope, and the key help with notes.
func (m model) chrome() int { return 3 + 2 + len(m.opts.Notes) + len(m.notes) }

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
	m.offset = max(min(m.offset, len(m.lines)-visible), 0)
}

// lineOf is the body line holding row i.
func (m model) lineOf(i int) int {
	if i >= 0 && i < len(m.rowLines) {
		return m.rowLines[i]
	}
	return 0
}

func (m *model) layoutRows() {
	m.lines = m.lines[:0]
	m.rowLines = make([]int, len(m.rows))
	m.harnessWidth = 0
	harnesses, entries := 0, 0
	for _, r := range m.rows {
		if r.harness != nil {
			harnesses++
			m.harnessWidth = max(m.harnessWidth, ansi.StringWidth(r.harness.Name))
		} else {
			entries++
			m.harnessWidth = max(m.harnessWidth, ansi.StringWidth(r.entry.Harness))
		}
	}
	m.harnessWidth = min(m.harnessWidth, 20)
	m.sessionCount = entries
	if harnesses > 0 {
		m.lines = append(m.lines, bodyLine{kind: harnessHeading})
	}
	for i, r := range m.rows {
		if r.entry != nil && i == harnesses {
			if harnesses > 0 {
				m.lines = append(m.lines, bodyLine{kind: blankLine})
			}
			m.lines = append(m.lines, bodyLine{kind: sessionHeading})
		}
		m.rowLines[i] = len(m.lines)
		m.lines = append(m.lines, bodyLine{kind: listRow, row: i})
	}
	if entries == 0 {
		if harnesses > 0 {
			m.lines = append(m.lines, bodyLine{kind: blankLine})
		}
		m.lines = append(m.lines, bodyLine{kind: sessionHeading}, bodyLine{kind: emptySessions})
	}
}

func (m model) renderLine(line bodyLine) string {
	switch line.kind {
	case harnessHeading:
		return m.render(m.styles.heading, "New session")
	case sessionHeading:
		return m.sessionsHeading(m.sessionCount)
	case emptySessions:
		text := "No sessions yet."
		switch {
		case m.loading:
			text = "Loading sessions…"
		case strings.TrimSpace(m.input.Value()) != "":
			text = "No matching sessions."
		case !m.all:
			text = "No sessions in this directory. Tab shows all."
		}
		return "  " + m.render(m.styles.muted, text)
	case listRow:
		row := m.rows[line.row]
		if row.entry != nil {
			return m.sessionRow(*row.entry, m.harnessWidth, line.row == m.cursor)
		}
		text := pad(row.harness.Name, m.harnessWidth)
		if row.harness.Builtin != row.harness.Name {
			text += "  " + m.render(m.styles.muted, row.harness.Builtin)
		}
		return m.line(text, line.row == m.cursor)
	default:
		return ""
	}
}

func (m model) sessionsHeading(count int) string {
	scope := "here"
	if m.all {
		scope = "all directories"
	}
	return m.render(m.styles.heading, "Sessions") + m.render(m.styles.muted, " · "+scope+" · "+strconv.Itoa(count))
}

func (m model) sessionRow(e sessions.Entry, harnessWidth int, selected bool) string {
	ago := pad(humanize.RelTime(e.Updated, m.now, "ago", "from now"), 3)
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
		e := &m.pickerSession
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
		end := min(m.offset+visible, len(m.lines))
		for _, line := range m.lines[m.offset:end] {
			b.WriteString(m.renderLine(line) + "\n")
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
	for _, note := range append(append([]string{}, m.opts.Notes...), m.notes...) {
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
