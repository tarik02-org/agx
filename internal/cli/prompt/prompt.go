// Package prompt asks for input with huh forms on stderr.
package prompt

import (
	"context"
	"errors"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/tarik02-org/agx/internal/cli/ui"
)

// ErrInterrupted reports a form aborted with Ctrl-C or Esc.
var ErrInterrupted = huh.ErrUserAborted

func run(ctx context.Context, field huh.Field) error {
	keys := huh.NewDefaultKeyMap()
	keys.Quit.SetKeys("ctrl+c", "esc")
	return huh.NewForm(huh.NewGroup(field)).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithKeyMap(keys).
		WithProgramOptions(tea.WithColorProfile(ui.Stderr.Profile())).
		RunWithContext(ctx)
}

// Input asks for a line of text; validate rejects answers until it returns nil.
func Input(ctx context.Context, label, placeholder string, validate func(string) error) (string, error) {
	var value string
	field := huh.NewInput().Title(label).Placeholder(placeholder).Value(&value).
		Validate(func(value string) error {
			if validate != nil {
				return validate(strings.TrimSpace(value))
			}
			return nil
		})
	err := run(ctx, field)
	return strings.TrimSpace(value), err
}

// Confirm asks a yes/no question; Enter takes the default.
func Confirm(ctx context.Context, question string, yes bool) (bool, error) {
	err := run(ctx, huh.NewConfirm().Title(question).Value(&yes))
	return yes, err
}

// Choose asks for one of options and returns its index.
func Choose(ctx context.Context, title string, options []string) (int, error) {
	choices := []huh.Option[int]{}
	for i, option := range options {
		choices = append(choices, huh.NewOption(option, i))
	}
	chosen := 0
	err := run(ctx, huh.NewSelect[int]().Title(title).Options(choices...).Value(&chosen))
	return chosen, err
}

// Item is one choice of a selection.
type Item struct {
	Label, Detail string
	Selected      bool
}

// Select asks for at least one item and returns the chosen indexes in order.
func Select(ctx context.Context, title string, items []Item) ([]int, error) {
	options := []huh.Option[int]{}
	for i, item := range items {
		text := item.Label
		if item.Detail != "" {
			text += "  " + item.Detail
		}
		options = append(options, huh.NewOption(text, i).Selected(item.Selected))
	}
	var chosen []int
	field := huh.NewMultiSelect[int]().Title(title).Options(options...).Filterable(true).
		Height(len(options) + 1).Value(&chosen).
		Validate(func(chosen []int) error {
			if len(chosen) == 0 {
				return errors.New("select at least one with x or space")
			}
			return nil
		})
	if err := run(ctx, field); err != nil {
		return nil, err
	}
	slices.Sort(chosen)
	return chosen, nil
}
