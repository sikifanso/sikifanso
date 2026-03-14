package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
)

// BrowseOpts configures the TUI browser.
type BrowseOpts struct {
	Entries []catalog.Entry
}

// item is a display-ready catalog entry with its original index.
type item struct {
	entry    catalog.Entry
	origIdx  int // index into the original entries slice
	category string
}

type model struct {
	entries     []catalog.Entry // original entries (mutable — toggles update here)
	items       []item          // visible items (filtered)
	cursor      int
	search      textinput.Model
	searching   bool
	toggled     map[string]bool // name→newEnabled for items changed from original
	origEnabled map[string]bool // snapshot of enabled state at TUI launch
	quitting    bool
}

func initialModel(opts BrowseOpts) model {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 64

	orig := make(map[string]bool, len(opts.Entries))
	for _, e := range opts.Entries {
		orig[e.Name] = e.Enabled
	}

	m := model{
		entries:     opts.Entries,
		search:      ti,
		toggled:     make(map[string]bool),
		origEnabled: orig,
	}
	m.items = m.buildItems("")
	return m
}

// buildItems creates the filtered and grouped item list.
func (m *model) buildItems(filter string) []item {
	filter = strings.ToLower(filter)

	// Group entries by category.
	cats := make(map[string][]int)
	for i, e := range m.entries {
		if filter != "" {
			if !strings.Contains(strings.ToLower(e.Name), filter) &&
				!strings.Contains(strings.ToLower(e.Description), filter) &&
				!strings.Contains(strings.ToLower(e.Category), filter) {
				continue
			}
		}
		cats[e.Category] = append(cats[e.Category], i)
	}

	// Sort categories.
	catNames := make([]string, 0, len(cats))
	for c := range cats {
		catNames = append(catNames, c)
	}
	sort.Strings(catNames)

	var items []item
	for _, cat := range catNames {
		for _, idx := range cats[cat] {
			items = append(items, item{
				entry:    m.entries[idx],
				origIdx:  idx,
				category: cat,
			})
		}
	}
	return items
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.searching {
		return m.updateSearch(msg)
	}
	return m.updateNormal(msg)
}

func (m model) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case key.Matches(msg, keys.Toggle):
			if len(m.items) > 0 {
				it := &m.items[m.cursor]
				newEnabled := !it.entry.Enabled
				m.entries[it.origIdx].Enabled = newEnabled
				it.entry.Enabled = newEnabled

				name := it.entry.Name
				if newEnabled == m.origEnabled[name] {
					delete(m.toggled, name) // back to original → unstage
				} else {
					m.toggled[name] = newEnabled // changed from original
				}
			}

		case key.Matches(msg, keys.Search):
			m.searching = true
			m.search.Focus()
			return m, m.search.Cursor.BlinkCmd()
		}
	}
	return m, nil
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Escape):
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
			m.items = m.buildItems("")
			m.cursor = 0
			return m, nil

		case msg.Type == tea.KeyEnter:
			m.searching = false
			m.search.Blur()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.items = m.buildItems(m.search.Value())
	if m.cursor >= len(m.items) {
		m.cursor = max(0, len(m.items)-1)
	}
	return m, cmd
}

// Browse launches the interactive TUI catalog browser.
// It returns a map of name→newEnabled for items that were changed from their
// original state. Returns nil if nothing was changed.
func Browse(opts BrowseOpts) (map[string]bool, error) {
	m := initialModel(opts)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := result.(model)
	if len(final.toggled) == 0 {
		return nil, nil
	}
	return final.toggled, nil
}
