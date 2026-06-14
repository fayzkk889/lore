package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sergi/go-diff/diffmatchpatch"
	"lore-cli/internal/display"
)

// ── Diff computation ──────────────────────────────────────────────────────────

type diffLine struct {
	kind rune // '+', '-', or ' '
	text string
}

// computeDiff returns a line-level edit script between oldText and newText
// using Myers' algorithm via sergi/go-diff.
func computeDiff(oldText, newText string) []diffLine {
	dmp := diffmatchpatch.New()

	// DiffLinesToChars rewrites both inputs as opaque rune sequences (one
	// rune per unique line) so DiffMain operates at line granularity.
	a, b, lineArr := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArr)

	var result []diffLine
	for _, d := range diffs {
		var kind rune
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			kind = '-'
		case diffmatchpatch.DiffInsert:
			kind = '+'
		default:
			kind = ' '
		}
		lines := strings.Split(d.Text, "\n")
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		for _, line := range lines {
			result = append(result, diffLine{kind: kind, text: line})
		}
	}
	return result
}

// ── Rendering ─────────────────────────────────────────────────────────────────

var (
	addStyle    = lipgloss.NewStyle().Foreground(display.ColorGreen)
	removeStyle = lipgloss.NewStyle().Foreground(display.ColorRed)
	ctxStyle    = lipgloss.NewStyle().Foreground(display.ColorGray)

	diffBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(display.ColorDimCyan).
			Padding(0, 1)
)

// maxDiffLines caps how many diff lines are rendered per file so huge
// generated files do not flood the transcript.
const maxDiffLines = 80

// renderFileDiff renders one bordered, readable diff for a file write.
func renderFileDiff(path, oldContent, newContent string, width int) string {
	isNew := oldContent == ""

	var lines []diffLine
	if isNew {
		for _, l := range strings.Split(strings.TrimRight(newContent, "\n"), "\n") {
			lines = append(lines, diffLine{kind: '+', text: l})
		}
	} else {
		lines = computeDiff(oldContent, newContent)
	}

	added, removed := 0, 0
	for _, l := range lines {
		switch l.kind {
		case '+':
			added++
		case '-':
			removed++
		}
	}

	label := display.SuccessStyle.Render("new")
	if !isNew {
		label = lipgloss.NewStyle().Foreground(display.ColorYellow).Render("edit")
	}
	header := fmt.Sprintf("%s %s  %s %s",
		label,
		display.BoldStyle.Render(path),
		addStyle.Render(fmt.Sprintf("+%d", added)),
		removeStyle.Render(fmt.Sprintf("-%d", removed)),
	)

	contentWidth := max(20, width-6)
	var sb strings.Builder
	sb.WriteString(header + "\n")

	shown := 0
	skippedCtx := 0
	for _, dl := range lines {
		if shown >= maxDiffLines {
			break
		}
		// In edits, compress long runs of unchanged context.
		if !isNew && dl.kind == ' ' {
			skippedCtx++
			if skippedCtx > 2 {
				continue
			}
		} else {
			if skippedCtx > 2 {
				sb.WriteString(ctxStyle.Render(fmt.Sprintf("  ⋮ %d unchanged lines", skippedCtx-2)) + "\n")
			}
			skippedCtx = 0
		}
		text := dl.text
		if len(text) > contentWidth {
			text = text[:contentWidth] + "…"
		}
		switch dl.kind {
		case '+':
			sb.WriteString(addStyle.Render("+ "+text) + "\n")
		case '-':
			sb.WriteString(removeStyle.Render("- "+text) + "\n")
		default:
			sb.WriteString(ctxStyle.Render("  "+text) + "\n")
		}
		shown++
	}
	if rest := len(lines) - shown; rest > 0 && shown >= maxDiffLines {
		sb.WriteString(display.DimStyle.Render(fmt.Sprintf("… %d more lines", rest)) + "\n")
	}

	return diffBoxStyle.Width(min(width-2, contentWidth+4)).Render(strings.TrimRight(sb.String(), "\n"))
}
