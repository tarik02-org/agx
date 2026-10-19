// Package prompt asks a person at a terminal for missing values: a line of text, a yes/no
// confirmation, or a multiple selection. Prompts are huh fields drawn inline on stderr in the
// launcher's styling, and leave the answer on screen.
package prompt

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
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
	question, answer, muted lipgloss.Style
	plain                   bool
}

func newStyles(profile colorprofile.Profile) styles {
	return styles{
		question: lipgloss.NewStyle().Bold(true),
		answer:   lipgloss.NewStyle().Foreground(lipgloss.Cyan),
		muted:    lipgloss.NewStyle().Faint(true),
		plain:    profile <= colorprofile.ASCII,
	}
}

func (s styles) render(style lipgloss.Style, text string) string {
	if s.plain || text == "" {
		return text
	}
	return style.Render(text)
}

const (
	selector         = "› "
	plainSelector    = "> "
	selectedPrefix   = "[x] "
	unselectedPrefix = "[ ] "
)

// theme mirrors the launcher: bold questions, cyan for what is chosen, faint for hints, and the
// terminal's own ANSI colors. Unstyled, it keeps only the marks that say where focus is.
func (s styles) theme() huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		var t huh.Styles
		t.Form.Base = lipgloss.NewStyle()
		t.Group.Base = lipgloss.NewStyle()
		t.FieldSeparator = lipgloss.NewStyle().SetString("\n\n")
		f := &t.Focused
		f.Base = lipgloss.NewStyle()
		f.Card = f.Base
		f.ErrorIndicator = lipgloss.NewStyle().SetString(" *")
		f.ErrorMessage = lipgloss.NewStyle()
		f.SelectSelector = lipgloss.NewStyle().SetString(plainSelector)
		f.MultiSelectSelector = f.SelectSelector
		f.SelectedPrefix = lipgloss.NewStyle().SetString(selectedPrefix)
		f.UnselectedPrefix = lipgloss.NewStyle().SetString(unselectedPrefix)
		// Buttons follow the question on its line.
		f.FocusedButton = lipgloss.NewStyle().MarginLeft(1).Transform(func(s string) string { return "[" + s + "]" })
		f.BlurredButton = lipgloss.NewStyle().MarginLeft(1).Transform(func(s string) string { return " " + s + " " })
		if !s.plain {
			f.Title = lipgloss.NewStyle().Bold(true)
			f.Description = lipgloss.NewStyle().Faint(true)
			f.ErrorIndicator = f.ErrorIndicator.Foreground(lipgloss.Red)
			f.ErrorMessage = f.ErrorMessage.Foreground(lipgloss.Red)
			f.SelectSelector = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Cyan).SetString(selector)
			f.MultiSelectSelector = f.SelectSelector
			f.SelectedPrefix = f.SelectedPrefix.Bold(true).Foreground(lipgloss.Cyan)
			f.SelectedOption = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
			f.UnselectedPrefix = f.UnselectedPrefix.Faint(true)
			f.FocusedButton = f.FocusedButton.Bold(true).Foreground(lipgloss.Cyan)
			f.BlurredButton = f.BlurredButton.Faint(true)
			f.TextInput.Cursor = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
			f.TextInput.Placeholder = lipgloss.NewStyle().Faint(true)
			muted := lipgloss.NewStyle().Faint(true)
			t.Help = help.Styles{
				Ellipsis:       muted,
				ShortKey:       muted,
				ShortDesc:      muted,
				ShortSeparator: muted,
				FullKey:        muted,
				FullDesc:       muted,
				FullSeparator:  muted,
			}
		}
		t.Blurred = t.Focused
		t.Group.Title = f.Title
		t.Group.Description = f.Description
		return &t
	})
}

// model runs one huh form and keeps the answer on screen once it ends, which huh alone clears.
type model struct {
	form *huh.Form
	// final is the line left behind; err is nil after a submission.
	final func(err error) string
	// press, when set, sees each key before the form does.
	press func(tea.KeyPressMsg)
	err   error
}

func (m model) Init() tea.Cmd { return m.form.Init() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch press.String() {
		case "ctrl+c":
			m.err = ErrInterrupted
			return m, tea.Quit
		case "esc":
			// Esc belongs to the field while it means something there, as leaving a filter does.
			if !claims(m.form.GetFocusedField(), press) {
				m.err = ErrCancelled
				return m, tea.Quit
			}
		}
		if m.press != nil {
			m.press(press)
		}
	}
	form, cmd := m.form.Update(msg)
	m.form = form.(*huh.Form)
	return m, cmd
}

func claims(field huh.Field, press tea.KeyPressMsg) bool {
	for _, binding := range field.KeyBinds() {
		if binding.Enabled() && key.Matches(press, binding) {
			return true
		}
	}
	return false
}

func (m model) View() tea.View {
	switch {
	case m.err != nil:
		return tea.NewView(m.final(m.err))
	case m.form.State == huh.StateCompleted:
		return tea.NewView(m.final(nil))
	}
	return tea.NewView(m.form.View())
}

func run(ctx context.Context, s styles, field huh.Field, showHelp bool, m model) error {
	form := huh.NewForm(huh.NewGroup(field)).WithTheme(s.theme()).WithShowHelp(showHelp)
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Quit
	profile := ui.Stderr.Profile()
	if s.plain {
		// Unstyled prompts still mark the cursor in reverse video; colors stay off.
		profile = colorprofile.ASCII
	}
	m.form = form
	result, err := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithOutput(os.Stderr),
		tea.WithColorProfile(profile),
	).Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) {
			return ErrInterrupted
		}
		return err
	}
	return result.(model).err
}

// Input asks for a line of text; validate, when set, rejects answers until it returns nil.
func Input(ctx context.Context, label, placeholder string, validate func(string) error) (string, error) {
	s := newStyles(ui.Stderr.Profile())
	var value string
	field := huh.NewInput().
		Title(label).
		Prompt(" ").
		Placeholder(placeholder).
		Inline(true).
		Value(&value).
		Validate(func(value string) error {
			if validate == nil {
				return nil
			}
			return validate(strings.TrimSpace(value))
		})
	err := run(ctx, s, field, false, model{final: func(error) string {
		return s.render(s.question, label) + " " + s.render(s.answer, strings.TrimSpace(value)) + "\n"
	}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

// Confirm asks a yes/no question; Enter takes the default.
func Confirm(ctx context.Context, question string, yes bool) (bool, error) {
	s := newStyles(ui.Stderr.Profile())
	field := huh.NewConfirm().Title(question).Inline(true).WithButtonAlignment(lipgloss.Left).Value(&yes)
	err := run(ctx, s, field, false, model{final: func(err error) string {
		line := s.render(s.question, question) + " "
		switch {
		case err != nil:
		case yes:
			line += s.render(s.answer, "yes")
		default:
			line += s.render(s.answer, "no")
		}
		return line + "\n"
	}})
	return yes, err
}

// Item is one choice of a selection.
type Item struct {
	Label, Detail string
	Selected      bool
}

// Select asks for any number of items, filtered with /; x or Space toggles one, Ctrl-A toggles
// all, and Enter accepts a selection of at least one. It returns the chosen indexes in order.
func Select(ctx context.Context, title string, items []Item) ([]int, error) {
	s := newStyles(ui.Stderr.Profile())
	width := ui.Stderr.Width()
	if width == 0 {
		width = 80
	}
	room := width - ansi.StringWidth(selector+selectedPrefix) - 1
	labelWidth := 0
	for _, item := range items {
		labelWidth = max(labelWidth, ansi.StringWidth(item.Label))
	}
	options := []huh.Option[int]{}
	for i, item := range items {
		// Option keys are what huh shows and filters, so a detail is both seen and searched.
		text := item.Label
		if item.Detail != "" {
			text += strings.Repeat(" ", labelWidth-ansi.StringWidth(item.Label)) + "  " +
				s.render(s.muted, item.Detail)
		}
		options = append(options, huh.NewOption(ansi.Truncate(text, max(room, 20), "…"), i).Selected(item.Selected))
	}
	var chosen []int
	// huh validates on every toggle; an empty selection is a problem only once Enter tries it.
	tried := false
	field := huh.NewMultiSelect[int]().
		Title(title).
		Options(options...).
		Filterable(true).
		// huh leaves the last option out of view when the height is left to it.
		Height(len(options) + 1).
		Value(&chosen).
		Validate(func(chosen []int) error {
			if tried && len(chosen) == 0 {
				return errors.New("select at least one with x or space")
			}
			return nil
		})
	press := func(press tea.KeyPressMsg) {
		tried = tried || press.String() == "enter" && !field.GetFiltering()
	}
	err := run(ctx, s, field, true, model{press: press, final: func(err error) string {
		if err != nil {
			return ""
		}
		labels := []string{}
		for _, i := range slices.Sorted(slices.Values(chosen)) {
			labels = append(labels, items[i].Label)
		}
		return s.render(s.question, title) + " " + s.render(s.answer, strings.Join(labels, ", ")) + "\n"
	}})
	if err != nil {
		return nil, err
	}
	return slices.Sorted(slices.Values(chosen)), nil
}
