// Package display — the LORE startup banner.
package display

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// bannerLines is the LORE wordmark, block style. 38 columns wide — fits any
// reasonable terminal.
var bannerLines = []string{
	`██╗      ██████╗ ██████╗ ███████╗`,
	`██║     ██╔═══██╗██╔══██╗██╔════╝`,
	`██║     ██║   ██║██████╔╝█████╗  `,
	`██║     ██║   ██║██╔══██╗██╔══╝  `,
	`███████╗╚██████╔╝██║  ██║███████╗`,
	`╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝`,
}

// bannerColors is a top-to-bottom cyan→violet gradient, one color per line.
var bannerColors = []string{"#22d3ee", "#38bdf8", "#60a5fa", "#818cf8", "#a78bfa", "#c084fc"}

// Banner renders the LORE wordmark with a vertical gradient and a tagline.
// version may be empty.
func Banner(version string) string {
	var b strings.Builder
	b.WriteString("\n")
	for i, line := range bannerLines {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(bannerColors[i%len(bannerColors)]))
		b.WriteString("  " + style.Render(line) + "\n")
	}
	tag := "open-source AI coding agent · bring your own key"
	if version != "" && version != "dev" {
		tag = "v" + strings.TrimPrefix(version, "v") + " · " + tag
	}
	b.WriteString("  " + DimStyle.Render(tag) + "\n")
	return b.String()
}

// BannerPlain renders the wordmark without ANSI colors (for --version and
// non-TTY output).
func BannerPlain(version string) string {
	var b strings.Builder
	b.WriteString("\n")
	for _, line := range bannerLines {
		b.WriteString("  " + line + "\n")
	}
	tag := "open-source AI coding agent · bring your own key"
	if version != "" && version != "dev" {
		tag = "v" + strings.TrimPrefix(version, "v") + " · " + tag
	}
	b.WriteString("  " + tag + "\n")
	return b.String()
}
