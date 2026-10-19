// Package ui renders human-facing CLI output. Each stream decides on its own whether it is
// styled; unstyled output keeps the same words and order so it stays stable for pipes and grep.
package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"
	"charm.land/lipgloss/v2/table"
	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

func (m *ColorMode) String() string { return string(*m) }

func (m *ColorMode) Set(value string) error {
	switch ColorMode(value) {
	case ColorAuto, ColorAlways, ColorNever:
		*m = ColorMode(value)
		return nil
	}
	return fmt.Errorf("must be one of auto, always, never")
}

func (m *ColorMode) Type() string { return "when" }

type Level int

const (
	Info Level = iota
	OK
	Warn
	Error
)

// Standard ANSI colors follow the user's terminal theme instead of assuming a background.
var (
	red     = lipgloss.Red
	green   = lipgloss.Green
	yellow  = lipgloss.Yellow
	blue    = lipgloss.Blue
	magenta = lipgloss.Magenta
	cyan    = lipgloss.Cyan
)

var (
	Stdout = New(os.Stdout, ColorAuto)
	Stderr = New(os.Stderr, ColorAuto)
)

// Configure applies the --color decision to both standard streams.
func Configure(mode ColorMode) {
	Stdout = New(os.Stdout, mode)
	Stderr = New(os.Stderr, mode)
}

type Printer struct {
	file    *os.File
	out     io.Writer
	profile colorprofile.Profile
	styled  bool
	tty     bool
	width   int
	enable  sync.Once
}

func New(f *os.File, mode ColorMode) *Printer {
	environ := os.Environ()
	tty := term.IsTerminal(f.Fd())
	profile := colorprofile.NoTTY
	switch mode {
	case ColorAlways:
		profile = max(colorprofile.Env(withoutNoColor(environ)), colorprofile.ANSI)
	case ColorAuto:
		// colorprofile keeps bold and faint under NO_COLOR; plain output must carry no escapes at all.
		if os.Getenv("NO_COLOR") == "" {
			profile = colorprofile.Detect(f, environ)
			if forced(os.Getenv("FORCE_COLOR")) {
				profile = max(profile, colorprofile.ANSI)
			}
		}
	case ColorNever:
	}
	p := &Printer{
		file:    f,
		out:     &colorprofile.Writer{Forward: f, Profile: profile},
		profile: profile,
		styled:  profile > colorprofile.ASCII,
		tty:     tty,
	}
	if tty {
		if w, _, err := term.GetSize(f.Fd()); err == nil && w > 0 {
			p.width = w
		}
	}
	return p
}

func withoutNoColor(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, pair := range environ {
		if !strings.HasPrefix(pair, "NO_COLOR=") {
			kept = append(kept, pair)
		}
	}
	return kept
}

func forced(value string) bool {
	return value != "" && value != "0" && value != "false"
}

// Write downsamples styled text to the stream's color profile, or strips it when unstyled.
func (p *Printer) Write(b []byte) (int, error) {
	if p.styled {
		p.enable.Do(func() { enableVirtualTerminal(p.file) })
	}
	return p.out.Write(b)
}

func (p *Printer) Styled() bool { return p.styled }

// Profile is the color profile the stream renders with, for full-screen programs drawing to it.
func (p *Printer) Profile() colorprofile.Profile { return p.profile }

// Terminal reports whether a person is likely watching, for output such as progress and hints.
func (p *Printer) Terminal() bool { return p.tty }

// Width is the terminal width, or 0 when the stream is not a terminal and must not be wrapped.
func (p *Printer) Width() int { return p.width }

func (p *Printer) render(style lipgloss.Style, text string) string {
	if !p.styled || text == "" {
		return text
	}
	return style.Render(text)
}

func (p *Printer) Heading(text string) string {
	return p.render(lipgloss.NewStyle().Bold(true), text)
}

func (p *Printer) Label(text string) string {
	return p.render(lipgloss.NewStyle().Bold(true), text)
}

func (p *Printer) Muted(text string) string {
	return p.render(lipgloss.NewStyle().Faint(true), text)
}

// ID marks identifiers users type back into commands: resources, launches, secrets, harnesses.
func (p *Printer) ID(text string) string {
	return p.render(lipgloss.NewStyle().Foreground(cyan), text)
}

func (p *Printer) Path(text string) string {
	return p.render(lipgloss.NewStyle().Foreground(blue), text)
}

// Value marks configured text such as "!secret id"; callers must never pass resolved secrets.
func (p *Printer) Value(text string) string {
	return p.render(lipgloss.NewStyle().Foreground(magenta), text)
}

// Syntax is the role of a token in highlighted code: tool signatures and JSON.
type Syntax int

const (
	SyntaxName Syntax = iota
	SyntaxKey
	SyntaxType
	SyntaxString
	SyntaxNumber
	SyntaxLiteral
	SyntaxPunctuation
)

var syntaxStyles = map[Syntax]lipgloss.Style{
	SyntaxName:        lipgloss.NewStyle().Bold(true).Foreground(cyan),
	SyntaxKey:         lipgloss.NewStyle().Foreground(blue),
	SyntaxType:        lipgloss.NewStyle().Foreground(yellow),
	SyntaxString:      lipgloss.NewStyle().Foreground(green),
	SyntaxNumber:      lipgloss.NewStyle().Foreground(magenta),
	SyntaxLiteral:     lipgloss.NewStyle().Foreground(magenta),
	SyntaxPunctuation: lipgloss.NewStyle().Faint(true),
}

// Code styles one token of highlighted code.
func (p *Printer) Code(role Syntax, text string) string {
	return p.render(syntaxStyles[role], text)
}

// Link makes a URL stand out and clickable; it stays one unwrapped line so it can be copied.
func (p *Printer) Link(url string) string {
	return p.render(lipgloss.NewStyle().Bold(true).Foreground(cyan).Hyperlink(url), url)
}

func (p *Printer) Badge(level Level, text string) string {
	style := lipgloss.NewStyle().Bold(true)
	switch level {
	case OK:
		style = style.Foreground(green)
	case Warn:
		style = style.Foreground(yellow)
	case Error:
		style = style.Foreground(red)
	case Info:
		style = lipgloss.NewStyle().Faint(true)
	}
	return p.render(style, text)
}

// Status renders a one-line outcome; the leading mark appears only when styled.
func (p *Printer) Status(level Level, text string) string {
	if !p.styled {
		return text
	}
	mark := map[Level]string{Info: "•", OK: "✓", Warn: "!", Error: "✗"}[level]
	return p.Badge(level, mark) + " " + text
}

// Diagnose writes "agx: <message>" to stderr with the prefix in the color of its level.
func Diagnose(level Level, format string, args ...any) {
	// A failed diagnostic write has nowhere left to be reported.
	_, _ = fmt.Fprintf(Stderr, "%s %s\n", Stderr.Badge(level, "agx:"), fmt.Sprintf(format, args...))
}

// Note writes a muted single line to stderr, such as a hint or progress.
func Note(text string) {
	_, _ = fmt.Fprintln(Stderr, Stderr.Muted(text))
}

// Ago describes how long before now t was, in the largest whole unit: "now", "5m", "3h", "2d",
// "6w", "1y". A time after now, as under clock skew, is "now".
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	case d < 14*24*time.Hour:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	case d < 365*24*time.Hour:
		return strconv.Itoa(int(d/(7*24*time.Hour))) + "w"
	}
	return strconv.Itoa(int(d/(365*24*time.Hour))) + "y"
}

// ShortPath writes a directory relative to cwd when it is cwd (".") or below it ("./sub"), and
// otherwise under the home directory as ~/rest.
func ShortPath(path, cwd string) string {
	if rel, err := filepath.Rel(cwd, path); err == nil && cwd != "" {
		if rel == "." {
			return "."
		}
		if filepath.IsLocal(rel) {
			return "." + string(filepath.Separator) + rel
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return filepath.Join("~", rest)
	}
	return path
}

// Fields renders aligned label/value pairs; empty values are skipped.
func (p *Printer) Fields(pairs ...[2]string) string {
	labelWidth := 0
	for _, pair := range pairs {
		if pair[1] != "" {
			labelWidth = max(labelWidth, len(pair[0]))
		}
	}
	var out strings.Builder
	for _, pair := range pairs {
		if pair[1] == "" {
			continue
		}
		label := pair[0] + strings.Repeat(" ", labelWidth-len(pair[0]))
		fmt.Fprintf(&out, "%s  %s\n", p.Muted(label), pair[1])
	}
	return out.String()
}

// Table renders rows under headers. Unstyled tables are space-aligned columns without borders.
// On a terminal, cells of the shrink columns lose their leading text when the table would
// overflow; identifiers are never cut, so they can still be copied.
func (p *Printer) Table(headers []string, rows [][]string, shrink ...int) string {
	if p.width > 0 && len(shrink) > 0 {
		rows = slices.Clone(rows)
		for i := range rows {
			rows[i] = slices.Clone(rows[i])
		}
		for _, col := range shrink {
			natural := lipgloss.Width(p.table(headers, rows))
			if natural <= p.width {
				break
			}
			widest := 0
			for _, row := range rows {
				widest = max(widest, lipgloss.Width(row[col]))
			}
			target := max(widest-(natural-p.width), len(headers[col]), 12)
			for _, row := range rows {
				if over := lipgloss.Width(row[col]) - target; over > 0 {
					row[col] = ansi.TruncateLeft(row[col], over+1, "…")
				}
			}
		}
	}
	lines := strings.Split(p.table(headers, rows), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func (p *Printer) table(headers []string, rows [][]string) string {
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(p.styled).
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Faint(true))
	last := len(headers) - 1
	t.StyleFunc(func(row, col int) lipgloss.Style {
		style := lipgloss.NewStyle()
		if col < last {
			style = style.PaddingRight(2)
		}
		if row == table.HeaderRow && p.styled {
			style = style.Bold(true)
		}
		return style
	})
	return t.Render()
}

// List renders one item per line, bulleted when styled and indented by two spaces otherwise.
func (p *Printer) List(items ...string) string {
	if len(items) == 0 {
		return ""
	}
	l := list.New()
	for _, item := range items {
		l.Item(item)
	}
	if p.styled {
		l.Enumerator(list.Bullet).EnumeratorStyle(lipgloss.NewStyle().Faint(true).PaddingLeft(2).PaddingRight(1))
	} else {
		l.Enumerator(func(list.Items, int) string { return "" }).EnumeratorStyle(lipgloss.NewStyle().PaddingLeft(2))
	}
	return l.String() + "\n"
}

// Tree returns a tree drawn with branch lines when styled, and indented by two spaces per level
// otherwise, so unstyled children read as "  child".
func (p *Printer) Tree() *tree.Tree {
	t := tree.New()
	if p.styled {
		return t.
			Enumerator(tree.RoundedEnumerator).
			EnumeratorStyle(lipgloss.NewStyle().Faint(true).PaddingRight(1)).
			IndenterStyle(lipgloss.NewStyle().Faint(true).PaddingRight(1))
	}
	return t.
		Enumerator(func(tree.Children, int) string { return "" }).
		Indenter(func(tree.Children, int) string { return "  " }).
		EnumeratorStyle(lipgloss.NewStyle()).
		IndenterStyle(lipgloss.NewStyle())
}

// Node returns a subtree for Tree with the same drawing settings.
func (p *Printer) Node(root string, children ...any) *tree.Tree {
	return p.Tree().Root(root).Child(children...)
}
