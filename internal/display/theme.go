// Package display provides shared lipgloss styles and colour tokens
// used across all Lore CLI commands.
package display

import "github.com/charmbracelet/lipgloss"

// Colour palette — all colours are defined once here to stay consistent.
var (
	ColorGreen    = lipgloss.Color("#00E676")
	ColorCyan     = lipgloss.Color("#00BCD4")
	ColorTeal     = lipgloss.Color("#00B8D4")
	ColorGray     = lipgloss.Color("#555555")
	ColorDimGray  = lipgloss.Color("#333333")
	ColorLightGray = lipgloss.Color("#888888")
	ColorRed      = lipgloss.Color("#FF5252")
	ColorWhite    = lipgloss.Color("#E0E0E0")
	ColorYellow   = lipgloss.Color("#FFD740")
	ColorPurple   = lipgloss.Color("#CE93D8")
	ColorDimCyan  = lipgloss.Color("#006064")
)

// Shared lipgloss styles.
var (
	// HeaderStyle draws a rounded green border suitable for the top banner.
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorGreen).
			Padding(0, 1)

	// PromptStyle renders the "you" input prefix in cyan — NO width or alignment
	// so that user messages are always left-aligned in the chat buffer.
	PromptStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	// MeterStyle renders the token usage line in muted gray italics.
	MeterStyle = lipgloss.NewStyle().
			Foreground(ColorGray).
			Italic(true)

	// ErrorStyle renders error messages in bold red.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	// SuccessStyle renders success messages in green.
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	// BoxStyle draws a rounded cyan border for info panels.
	BoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(0, 1)

	// DimStyle renders secondary text in muted gray.
	DimStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	// BoldStyle renders prominent labels in bold white.
	BoldStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWhite)

	// VersionStyle renders version strings in yellow.
	VersionStyle = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true)

	// AccentStyle renders accented secondary values in purple.
	AccentStyle = lipgloss.NewStyle().
			Foreground(ColorPurple)

	// TealStyle renders text in teal/cyan — used for the logo and header borders.
	TealStyle = lipgloss.NewStyle().
			Foreground(ColorTeal).
			Bold(true)

	// InputBoxStyle draws the rounded bordered chat input box in light gray.
	InputBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorLightGray).
			Padding(0, 1)

	// ResponseSepStyle renders the subtle inter-response separator in very dim gray.
	ResponseSepStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#444444"))
)

// InputBoxWaitingStyle is the input box border when waiting for API response.
var InputBoxWaitingStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#333333")).
	Padding(0, 1)

// InputBoxSuccessStyle is the input box border briefly after a successful response.
var InputBoxSuccessStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#00E676")).
	Padding(0, 1)

// DimCyanStyle renders text in a subtle dark cyan for accents.
var DimCyanStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#006064"))
