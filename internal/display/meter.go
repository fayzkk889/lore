// Package display — token meter that tracks per-session usage.
package display

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TokenMeter accumulates token usage across a single chat session. It is a
// plain count for the user's own awareness — Lore does no metering or
// billing; whatever the tokens cost is between the user and their provider.
type TokenMeter struct {
	// Session totals
	SessionInputTokens  int
	SessionOutputTokens int

	// Counts for the most recently completed exchange
	LastInputTokens  int
	LastOutputTokens int
}

// Add records the token counts from one API exchange.
func (m *TokenMeter) Add(input, output int) {
	m.LastInputTokens = input
	m.LastOutputTokens = output
	m.SessionInputTokens += input
	m.SessionOutputTokens += output
}

// TotalSessionTokens returns the combined input+output count for the session.
func (m *TokenMeter) TotalSessionTokens() int {
	return m.SessionInputTokens + m.SessionOutputTokens
}

// Display returns a single styled line summarising provider-reported usage.
// Format: turn billed: 4,201 | session billed: 12,847 (in 10,002 / out 2,845)
func (m *TokenMeter) Display() string {
	dimWhite := lipgloss.NewStyle().Foreground(ColorLightGray)

	last := m.LastInputTokens + m.LastOutputTokens
	tokStr := "~"
	if last > 0 {
		tokStr = FormatCommas(last)
	}

	return fmt.Sprintf("%s %s %s %s %s %s",
		dimWhite.Render("turn billed:"),
		dimWhite.Render(tokStr),
		dimWhite.Render("|"),
		dimWhite.Render("session billed:"),
		dimWhite.Render(FormatCommas(m.TotalSessionTokens())),
		dimWhite.Render(fmt.Sprintf("(in %s / out %s)",
			FormatCommas(m.SessionInputTokens), FormatCommas(m.SessionOutputTokens))),
	)
}

// SessionSummary returns a multi-line plain-text summary for the exit screen.
func (m *TokenMeter) SessionSummary(d time.Duration) string {
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf(
		"  Duration:       %dm %ds\n  Billed tokens:  %s  (in: %s  out: %s)",
		mins, secs,
		FormatCommas(m.TotalSessionTokens()),
		FormatCommas(m.SessionInputTokens),
		FormatCommas(m.SessionOutputTokens),
	)
}

// FormatCommas formats an integer with thousands separators (e.g. 12847 → "12,847").
// Exported so other packages (cmd) can reuse it.
func FormatCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(ch))
	}
	return string(out)
}
