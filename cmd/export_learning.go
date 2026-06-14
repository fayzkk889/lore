package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/signintech/gopdf"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/fayzkk889/lore/internal/display"
	"github.com/fayzkk889/lore/internal/fonts"
)

// ── Page geometry constants (A4 in points, 1pt = 1/72 inch) ──────────────────

const (
	pageW        = 595.28 // A4 width
	pageH        = 841.89 // A4 height
	marginLeft   = 56.0
	marginRight  = 56.0
	marginTop    = 60.0
	marginBottom = 60.0
	contentWidth = pageW - marginLeft - marginRight

	fontBody = "Inter"
	fontBold = "InterBold"
	fontMono = "JetBrainsMono"

	sizeTitle    = 26.0
	sizeSubtitle = 10.0
	sizeH1       = 22.0
	sizeH2       = 17.0
	sizeH3       = 14.0
	sizeHSmall   = 12.0
	sizeBody     = 11.5
	sizeMono     = 9.5

	lineH         = 17.0 // body line height
	lineHMono     = 14.5 // mono line height
	paraGap       = 8.0  // gap after a paragraph/block
	headingGapPre = 10.0 // extra space before a heading
)

// ── learningDoc holds parsed metadata for one .md file ───────────────────────

type learningDoc struct {
	fileName string // e.g. "understanding-rate-limiting.md"
	slug     string // without .md
	topic    string
	date     string
	session  string
	related  string
	body     string // markdown content with front-matter stripped
}

// ── Cobra command ─────────────────────────────────────────────────────────────

func newExportLearningCmd() *cobra.Command {
	var (
		flagAll    bool
		flagOutput string
	)

	cmd := &cobra.Command{
		Use:   "export-learning [slug]",
		Short: "Export learning documents as PDF",
		Long: `Export one or all learning documents from .lore/learnings/ as PDF files.

Examples:
  lore export-learning understanding-rate-limiting
  lore export-learning --all
  lore export-learning --all --output ./exports`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExportLearning(args, flagAll, flagOutput)
		},
	}

	cmd.Flags().BoolVar(&flagAll, "all", false, "Export all learnings as a single combined PDF")
	cmd.Flags().StringVarP(&flagOutput, "output", "o", ".", "Directory to write PDF file(s) into")

	return cmd
}

// ── Entry point ───────────────────────────────────────────────────────────────

func runExportLearning(args []string, all bool, outputDir string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	learningsDir := filepath.Join(cwd, ".lore", "learnings")
	if _, err := os.Stat(learningsDir); os.IsNotExist(err) {
		return fmt.Errorf(".lore/learnings/ not found — run `lore init` first")
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if all {
		docs, err := loadAllLearnings(learningsDir)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			fmt.Println(display.DimStyle.Render("No learning documents found in .lore/learnings/"))
			return nil
		}
		date := time.Now().Format("2006-01-02")
		outFile := filepath.Join(outputDir, "lore-learnings-"+date+".pdf")
		if err := exportCombinedPDF(docs, outFile); err != nil {
			return fmt.Errorf("generating combined PDF: %w", err)
		}
		fmt.Printf("Exported: %s\n", filepath.Base(outFile))
		return nil
	}

	if len(args) == 0 {
		return fmt.Errorf("provide a topic slug or use --all\n  Example: lore export-learning understanding-rate-limiting")
	}

	slug := strings.TrimSuffix(args[0], ".md")
	doc, err := loadLearning(learningsDir, slug)
	if err != nil {
		return err
	}

	outFile := filepath.Join(outputDir, slug+".pdf")
	if err := exportSinglePDF(doc, outFile); err != nil {
		return fmt.Errorf("generating PDF: %w", err)
	}
	fmt.Printf("Exported: %s\n", filepath.Base(outFile))
	return nil
}

// ── File loaders ──────────────────────────────────────────────────────────────

func loadAllLearnings(dir string) ([]learningDoc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading learnings directory: %w", err)
	}
	var docs []learningDoc
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		doc, err := loadLearning(dir, slug)
		if err != nil {
			fmt.Printf("  ⚠  skipping %s: %v\n", e.Name(), err)
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func loadLearning(dir, slug string) (learningDoc, error) {
	fileName := slug + ".md"
	path := filepath.Join(dir, fileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return learningDoc{}, fmt.Errorf("learning %q not found in .lore/learnings/", slug)
		}
		return learningDoc{}, fmt.Errorf("reading %s: %w", fileName, err)
	}

	content := string(raw)
	topic, body := extractFrontMatterAndBody(content)

	return learningDoc{
		fileName: fileName,
		slug:     slug,
		topic:    topic,
		date:     parseFrontMatterField(content, "date"),
		session:  parseFrontMatterField(content, "session"),
		related:  parseFrontMatterField(content, "related"),
		body:     body,
	}, nil
}

// extractFrontMatterAndBody splits YAML front-matter from the markdown body.
// Returns (topic, body). The topic is read from the front-matter "topic" field;
// if missing, it falls back to the first heading in the body.
func extractFrontMatterAndBody(content string) (topic, body string) {
	lines := strings.Split(content, "\n")

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		// Find closing ---
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmBlock := strings.Join(lines[:i+1], "\n")
				topic = parseFrontMatterField(fmBlock, "topic")
				body = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
				return topic, body
			}
		}
	}

	// No front-matter — use entire content as body.
	return "", strings.TrimSpace(content)
}

// ── PDF generation ────────────────────────────────────────────────────────────

// pdfWriter wraps gopdf and tracks the current Y cursor position.
type pdfWriter struct {
	pdf     *gopdf.GoPdf
	y       float64
	pageNum int
}

func newPDFWriter() (*pdfWriter, error) {
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: gopdf.Rect{W: pageW, H: pageH}})

	// Load embedded fonts.
	if err := p.AddTTFFontByReader(fontBody, bytes.NewReader(fonts.InterRegular)); err != nil {
		return nil, fmt.Errorf("loading Inter-Regular font: %w", err)
	}
	if err := p.AddTTFFontByReader(fontBold, bytes.NewReader(fonts.InterBold)); err != nil {
		return nil, fmt.Errorf("loading Inter-Bold font: %w", err)
	}
	if err := p.AddTTFFontByReader(fontMono, bytes.NewReader(fonts.JetBrainsMonoRegular)); err != nil {
		return nil, fmt.Errorf("loading JetBrainsMono-Regular font: %w", err)
	}

	p.SetTextColor(30, 30, 30) // near-black default
	return &pdfWriter{pdf: p}, nil
}

// addPage adds a new page and resets the Y cursor.
func (w *pdfWriter) addPage() {
	w.pdf.AddPage()
	w.pageNum++
	w.y = marginTop
}

// ensureSpace checks if at least `h` points remain on the page; if not, adds a new page.
func (w *pdfWriter) ensureSpace(h float64) {
	if w.y+h > pageH-marginBottom {
		w.addPage()
	}
}

// setFont is a convenience wrapper.
func (w *pdfWriter) setFont(name string, size float64) {
	_ = w.pdf.SetFont(name, "", size)
}

// writeLine writes one line of text at (marginLeft, w.y) and advances w.y.
func (w *pdfWriter) writeLine(text string, lineHeight float64) {
	w.pdf.SetXY(marginLeft, w.y)
	_ = w.pdf.Cell(nil, text)
	w.y += lineHeight
}

// writeWrapped word-wraps text across the full content width, drawing each
// line and advancing w.y. Returns the number of lines written.
func (w *pdfWriter) writeWrapped(content string, lineHeight float64) int {
	words := strings.Fields(content)
	if len(words) == 0 {
		w.y += lineHeight
		return 1
	}

	linesOut := 0
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		cw, _ := w.pdf.MeasureTextWidth(candidate)
		if cw > contentWidth && current != "" {
			w.ensureSpace(lineHeight)
			w.writeLine(current, lineHeight)
			linesOut++
			current = word
		} else {
			current = candidate
		}
	}
	if current != "" {
		w.ensureSpace(lineHeight)
		w.writeLine(current, lineHeight)
		linesOut++
	}
	return linesOut
}

// drawRule draws a thin horizontal rule at the current Y and advances.
func (w *pdfWriter) drawRule(thickness float64) {
	w.pdf.SetLineWidth(thickness)
	w.pdf.SetStrokeColor(180, 180, 180)
	w.pdf.Line(marginLeft, w.y, pageW-marginRight, w.y)
	w.y += 8
}

// drawCodeBlock renders a code block with a light-grey background tint.
func (w *pdfWriter) drawCodeBlock(code string) {
	w.setFont(fontMono, sizeMono)
	lines := strings.Split(code, "\n")
	// Remove trailing blank lines.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	blockH := float64(len(lines))*lineHMono + 10
	w.ensureSpace(blockH)

	// Draw background rectangle.
	w.pdf.SetFillColor(245, 245, 245)
	w.pdf.RectFromUpperLeftWithStyle(marginLeft, w.y, contentWidth, blockH, "F")
	w.pdf.SetTextColor(50, 50, 50) // dark grey for code
	w.y += 5

	for _, line := range lines {
		w.pdf.SetXY(marginLeft+6, w.y)
		// Truncate very long lines to avoid overflow (rare but defensive).
		if len(line) > 120 {
			line = line[:117] + "..."
		}
		_ = w.pdf.Cell(nil, line)
		w.y += lineHMono
	}
	w.pdf.SetTextColor(30, 30, 30) // reset to body color
	w.y += 5
}

// ── Cover / header for a single learning ─────────────────────────────────────

func (w *pdfWriter) writeLearningHeader(doc learningDoc) {
	// Title.
	w.setFont(fontBold, sizeTitle)
	w.writeWrapped(doc.topic, sizeTitle+6)
	w.y += 4

	// Subtitle line: date · session N.
	w.setFont(fontBody, sizeSubtitle)
	var meta []string
	if doc.date != "" {
		meta = append(meta, doc.date)
	}
	if doc.session != "" {
		meta = append(meta, "Session "+doc.session)
	}
	if doc.related != "" && doc.related != "[]" {
		meta = append(meta, "Related: "+doc.related)
	}
	if len(meta) > 0 {
		w.writeLine(strings.Join(meta, "  ·  "), sizeSubtitle+5)
	}

	w.drawRule(0.5)
	w.y += 4
}

// parseFrontMatterField extracts a "key: value" line from YAML front matter.
func parseFrontMatterField(content, key string) string {
	prefix := key + ": "
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// ── Goldmark AST renderer ─────────────────────────────────────────────────────

// renderMarkdownToPDF parses the markdown body and writes it into the pdfWriter.
func (w *pdfWriter) renderMarkdownToPDF(body string) {
	src := []byte(body)
	reader := text.NewReader(src)
	parser := goldmark.DefaultParser()
	doc := parser.Parse(reader)

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {

		case *ast.Heading:
			level := node.Level
			var sz float64
			switch level {
			case 1:
				sz = sizeH1
			case 2:
				sz = sizeH2
			case 3:
				sz = sizeH3
			default:
				sz = sizeHSmall
			}
			// Collect text from children.
			txt := extractTextFromNode(node, src)
			if txt == "" {
				return ast.WalkSkipChildren, nil
			}
			w.y += headingGapPre
			w.ensureSpace(sz + lineH)
			w.setFont(fontBold, sz)
			w.writeWrapped(txt, sz+5)
			w.y += 4
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			txt := extractTextFromNode(node, src)
			if txt == "" {
				return ast.WalkSkipChildren, nil
			}
			w.ensureSpace(lineH * 2)
			w.setFont(fontBody, sizeBody)
			w.writeWrapped(txt, lineH)
			w.y += paraGap
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock, *ast.CodeBlock:
			var code string
			if fc, ok := node.(*ast.FencedCodeBlock); ok {
				code = string(fc.Text(src))
			} else if cb, ok := node.(*ast.CodeBlock); ok {
				code = string(cb.Text(src))
			}
			w.y += 4
			w.drawCodeBlock(strings.TrimRight(code, "\n"))
			w.y += paraGap
			return ast.WalkSkipChildren, nil

		case *ast.List:
			// Handled by ListItem below.
			return ast.WalkContinue, nil

		case *ast.ListItem:
			txt := extractListItemText(node, src)
			if txt == "" {
				return ast.WalkSkipChildren, nil
			}
			w.ensureSpace(lineH + 2)
			w.setFont(fontBody, sizeBody)
			// Write bullet.
			w.pdf.SetXY(marginLeft, w.y)
			_ = w.pdf.Cell(nil, "•")
			origX := marginLeft
			// Word-wrap within adjusted width.
			words := strings.Fields(txt)
			current := ""
			for _, word := range words {
				candidate := word
				if current != "" {
					candidate = current + " " + word
				}
				cw, _ := w.pdf.MeasureTextWidth(candidate)
				limit := contentWidth - 12
				if cw > limit && current != "" {
					w.pdf.SetXY(origX+12, w.y)
					_ = w.pdf.Cell(nil, current)
					w.y += lineH
					w.pdf.SetXY(origX+12, w.y)
					current = word
				} else {
					current = candidate
				}
			}
			if current != "" {
				w.pdf.SetXY(origX+12, w.y)
				_ = w.pdf.Cell(nil, current)
				w.y += lineH
			}
			w.y += 3
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			w.y += 8
			w.drawRule(0.3)
			return ast.WalkContinue, nil

		case *ast.Blockquote:
			txt := extractTextFromNode(node, src)
			if txt != "" {
				w.y += 4
				w.setFont(fontBody, sizeBody)
				// Left accent line.
				w.pdf.SetLineWidth(3)
				w.pdf.SetStrokeColor(180, 180, 180)
				startY := w.y
				w.pdf.SetXY(marginLeft+8, w.y)
				w.writeWrapped(txt, lineH)
				w.pdf.Line(marginLeft, startY, marginLeft, w.y)
				w.y += paraGap
			}
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
}

// extractTextFromNode concatenates all text segments under a node.
func extractTextFromNode(n ast.Node, src []byte) string {
	var buf strings.Builder
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				buf.WriteByte(' ')
			}
		}
		if child.Kind() == ast.KindString {
			if s, ok := child.(*ast.String); ok {
				buf.Write(s.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

// extractListItemText collects paragraph text directly inside a ListItem node.
func extractListItemText(item ast.Node, src []byte) string {
	var parts []string
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*ast.List); ok {
			continue // skip nested lists for simplicity
		}
		t := extractTextFromNode(child, src)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

// ── Single PDF export ─────────────────────────────────────────────────────────

func exportSinglePDF(doc learningDoc, outPath string) error {
	w, err := newPDFWriter()
	if err != nil {
		return err
	}

	w.addPage()
	w.writeLearningHeader(doc)
	w.renderMarkdownToPDF(doc.body)

	// Footer: page number on every page.
	writePageNumbers(w)

	return w.pdf.WritePdf(outPath)
}

// ── Combined PDF export with TOC ─────────────────────────────────────────────

func exportCombinedPDF(docs []learningDoc, outPath string) error {
	w, err := newPDFWriter()
	if err != nil {
		return err
	}

	// ── TOC page ─────────────────────────────────────────────────────────────
	w.addPage()

	w.setFont(fontBold, sizeTitle)
	w.writeLine("Table of Contents", sizeTitle+8)
	w.drawRule(0.5)
	w.y += 4

	// We need to know page numbers ahead of time. Estimate pages per doc.
	// Strategy: render each doc to a scratch writer, count pages, then
	// render the real combined PDF. For simplicity we do a two-pass approach:
	// first pass records page numbers, second pass writes the real PDF.
	pageStarts := estimatePageStarts(docs)

	w.setFont(fontBody, sizeBody)
	for i, doc := range docs {
		topic := doc.topic
		if topic == "" {
			topic = doc.slug
		}
		pageRef := fmt.Sprintf("%d", pageStarts[i]+1) // TOC is page 1, content starts at 2
		// Dot-fill between title and page number.
		tw, _ := w.pdf.MeasureTextWidth(topic)
		pw, _ := w.pdf.MeasureTextWidth(pageRef)
		dotWidth, _ := w.pdf.MeasureTextWidth(".")
		available := contentWidth - tw - pw - 8
		dots := ""
		if dotWidth > 0 {
			n := int(available / dotWidth)
			if n > 0 {
				dots = strings.Repeat(".", n)
			}
		}
		line := fmt.Sprintf("%d. %s %s %s", i+1, topic, dots, pageRef)
		w.ensureSpace(lineH)
		w.writeLine(line, lineH+3)
	}

	// ── Content pages ─────────────────────────────────────────────────────────
	for _, doc := range docs {
		w.addPage()
		w.writeLearningHeader(doc)
		w.renderMarkdownToPDF(doc.body)
	}

	writePageNumbers(w)
	return w.pdf.WritePdf(outPath)
}

// estimatePageStarts does a lightweight content-height estimate to predict
// what page each learning starts on (for TOC page numbers). Page 1 = TOC.
func estimatePageStarts(docs []learningDoc) []int {
	starts := make([]int, len(docs))
	currentPage := 1 // page 1 = TOC
	for i, doc := range docs {
		currentPage++ // each doc gets at least one new page
		starts[i] = currentPage

		// Rough line count for body.
		bodyLines := len(strings.Split(doc.body, "\n"))
		availH := pageH - marginTop - marginBottom - 80.0
		linesPerPage := int(availH / lineH)
		if linesPerPage < 1 {
			linesPerPage = 40
		}
		extraPages := bodyLines / linesPerPage
		currentPage += extraPages
	}
	return starts
}

// writePageNumbers adds a centered page number footer to every page.
func writePageNumbers(w *pdfWriter) {
	total := w.pdf.GetNumberOfPages()
	for i := 1; i <= total; i++ {
		w.pdf.SetPage(i)
		_ = w.pdf.SetFont(fontBody, "", sizeSubtitle-1)
		label := fmt.Sprintf("%d / %d", i, total)
		lw, _ := w.pdf.MeasureTextWidth(label)
		w.pdf.SetXY((pageW-lw)/2, pageH-marginBottom+14)
		_ = w.pdf.Cell(nil, label)
	}
}
