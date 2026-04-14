package output

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	ColorCyan    = lipgloss.Color("6")
	ColorGreen   = lipgloss.Color("2")
	ColorYellow  = lipgloss.Color("3")
	ColorRed     = lipgloss.Color("1")
	ColorMagenta = lipgloss.Color("5")
	ColorDim     = lipgloss.Color("8")

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan)

	Subtitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorMagenta)

	Label = lipgloss.NewStyle().
		Foreground(ColorDim)

	Value = lipgloss.NewStyle()

	Success = lipgloss.NewStyle().
		Foreground(ColorGreen)

	Warning = lipgloss.NewStyle().
		Foreground(ColorYellow)

	Error = lipgloss.NewStyle().
		Foreground(ColorRed)

	Dim = lipgloss.NewStyle().
		Foreground(ColorDim)

	// Box for sections
	SectionBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDim).
			Padding(0, 1)

	// Indented block
	Indent = lipgloss.NewStyle().
		PaddingLeft(2)
)

// KeyValue formats a label: value pair with consistent alignment.
func KeyValue(label, value string, width int) string {
	l := Label.Render(label)
	v := Value.Render(value)
	padding := ""
	if width > len(label) {
		padding = spaces(width - len(label))
	}
	return l + padding + " " + v
}

func spaces(n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = ' '
	}
	return string(s)
}
