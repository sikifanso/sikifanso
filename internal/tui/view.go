package tui

import (
	"fmt"
	"strings"
)

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("AI Agent Infrastructure Catalog"))
	b.WriteString("\n")

	// Search bar.
	if m.searching {
		b.WriteString(searchStyle.Render("Search: "))
		b.WriteString(m.search.View())
		b.WriteString("\n\n")
	} else if m.search.Value() != "" {
		b.WriteString(searchStyle.Render(fmt.Sprintf("Filter: %s", m.search.Value())))
		b.WriteString("\n\n")
	}

	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render("  No matching apps"))
		b.WriteString("\n")
	} else {
		prevCat := ""
		for i, it := range m.items {
			// Category header.
			if it.category != prevCat {
				if prevCat != "" {
					b.WriteString("\n")
				}
				b.WriteString(categoryStyle.Render(fmt.Sprintf("  %s", it.category)))
				b.WriteString("\n")
				prevCat = it.category
			}

			// Cursor.
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			// Enabled indicator.
			var status string
			if _, isToggled := m.toggled[it.entry.Name]; isToggled {
				status = stagedStyle.Render("[staged]")
			} else if it.entry.Enabled {
				status = enabledStyle.Render(fmt.Sprintf("%-8s", "[on]"))
			} else {
				status = disabledStyle.Render(fmt.Sprintf("%-8s", "[off]"))
			}

			// Entry line.
			name := it.entry.Name
			desc := dimStyle.Render(it.entry.Description)
			ver := dimStyle.Render(it.entry.TargetRevision)

			line := fmt.Sprintf("%s  %-22s %-30s %s %s", cursor, name, desc, ver, status)
			if i == m.cursor {
				line = selectedStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString(helpStyle.Render("  [Enter] Toggle  [/] Search  [q] Quit"))
	b.WriteString("\n")

	return b.String()
}
