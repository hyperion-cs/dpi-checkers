package primitives

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

var (
	upKeys    = []string{"up", "k", "л", "ctrl+p", "ctrl+з"}
	downKeys  = []string{"down", "j", "о", "ctrl+n", "ctrl+т"}
	leftKeys  = []string{"left", "h", "р", "ctrl+b", "ctrl+и"}
	rightKeys = []string{"right", "l", "д", "ctrl+f", "ctrl+а"}
)

func TableOuterBorderStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))
}

func TableStyle(selectedActive bool) table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)

	s.Selected = s.Selected.
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("240")).
		Bold(false)

	if selectedActive {
		s.Selected = s.Selected.
			Background(lipgloss.Color("57"))
	}
	return s
}

func TableCellMaxLen(rows []table.Row, pos, min int) int {
	max := min
	for _, v := range rows {
		w := lipgloss.Width(v[pos])
		if w > max {
			max = w
		}
	}
	return max
}

func TableHeight(rows []table.Row, maxVisibleRows int) int {
	const extraHeight = 2 // internal table extra height
	return min(maxVisibleRows, len(rows)) + extraHeight
}

func TableWidth(cols []table.Column) int {
	const extraWidth = 2 // internal column extra width
	width := extraWidth * len(cols)
	for _, col := range cols {
		width += col.Width
	}
	return width
}

func TerminalRow(row table.Row) table.Row {
	for i, cell := range row {
		row[i] = TerminalText(cell)
	}
	return row
}

// NormKey translates alternate navigation bindings into arrow keys.
func NormKey(s string) string {
	switch {
	case slices.Contains(upKeys, s):
		return "up"
	case slices.Contains(downKeys, s):
		return "down"
	case slices.Contains(leftKeys, s):
		return "left"
	case slices.Contains(rightKeys, s):
		return "right"
	}

	return s
}

func TableKeyMap() table.KeyMap {
	km := table.DefaultKeyMap()

	km.LineUp = key.NewBinding(
		key.WithKeys(upKeys...),
		key.WithHelp("↑/k", "up"),
	)

	km.LineDown = key.NewBinding(
		key.WithKeys(downKeys...),
		key.WithHelp("↓/j", "down"),
	)

	km.HalfPageDown = key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "½ page down"),
	)

	return km
}
