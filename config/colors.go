package config

// Horizon dark theme palette.
// All values are hex color strings compatible with lipgloss.Color().
// To retheme Lira, update the constants below — nothing else needs changing.
const (
	// Base surface colors
	ColorBackground = "#1C1E26"
	ColorSurface    = "#232530"
	ColorBorder     = "#2E303E"
	ColorMuted      = "#6C6F93"

	// Text
	ColorForeground = "#E0E0E0"
	ColorSubtle     = "#A0A0B0"

	// Status
	ColorTodo       = "#6C6F93" // muted — not yet started
	ColorInProgress = "#FAB795" // warm orange
	ColorDone       = "#29D398" // green

	// Priority indicators
	ColorPriorityLow    = "#59E1E3" // cyan
	ColorPriorityMedium = "#FAB795" // orange
	ColorPriorityHigh   = "#E95678" // red

	// UI chrome
	ColorAccent    = "#26BBD9" // blue — selected item highlight
	ColorCursor    = "#EE64AC" // pink — active cursor
	ColorTitle     = "#E95678" // red — panel headers
	ColorKeybind   = "#59E1E3" // cyan — keybinding hints
)

// GoalColors is the ordered palette of colors available when creating a Goal.
// Colors are assigned in rotation when the user does not pick one manually.
// The string values match the named constants above for cross-referencing,
// but are duplicated here so the slice is self-contained for the TUI picker.
var GoalColors = []GoalColor{
	{Name: "rose",   Hex: "#E95678"},
	{Name: "peach",  Hex: "#F09383"},
	{Name: "gold",   Hex: "#FAB795"},
	{Name: "mint",   Hex: "#29D398"},
	{Name: "sky",    Hex: "#59E1E3"},
	{Name: "blue",   Hex: "#26BBD9"},
	{Name: "orchid", Hex: "#EE64AC"},
	{Name: "lavender", Hex: "#B877DB"},
}

// GoalColor pairs a human-readable name with its hex value.
type GoalColor struct {
	Name string
	Hex  string
}

// GoalColorByName returns the GoalColor matching name, and whether it was found.
func GoalColorByName(name string) (GoalColor, bool) {
	for _, c := range GoalColors {
		if c.Name == name {
			return c, true
		}
	}
	return GoalColor{}, false
}

// NextAutoColor returns the color that should be auto-assigned to the nth goal
// (0-indexed), wrapping around the palette.
func NextAutoColor(n int) GoalColor {
	return GoalColors[n%len(GoalColors)]
}
