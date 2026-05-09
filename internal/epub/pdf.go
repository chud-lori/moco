package epub

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	pdfReader "github.com/ledongthuc/pdf"
)

// PDFConverter names a converter that's been chosen to handle a PDF.
type PDFConverter string

const (
	ConverterEbookConvert PDFConverter = "ebook-convert" // Calibre — best
	ConverterMutool       PDFConverter = "mutool"        // MuPDF — text + images
	ConverterPdftotext    PDFConverter = "pdftotext"     // Poppler — text only
	ConverterPureGo       PDFConverter = "pure-go"       // Last resort
	ConverterUnavailable  PDFConverter = ""
)

// DetectPDFConverter returns the highest-fidelity PDF converter available on
// the host PATH. Pure-Go is always available so this function never returns
// the empty string in normal builds.
func DetectPDFConverter() PDFConverter {
	if _, err := exec.LookPath("ebook-convert"); err == nil {
		return ConverterEbookConvert
	}
	if _, err := exec.LookPath("mutool"); err == nil {
		return ConverterMutool
	}
	if _, err := exec.LookPath("pdftotext"); err == nil {
		return ConverterPdftotext
	}
	return ConverterPureGo
}

// PDFToEPUB converts a PDF on disk to an EPUB byte slice. It walks the
// converter chain (ebook-convert → mutool → pdftotext → pure-Go) and returns
// the first successful output. Each tier preserves more fidelity than the next.
func PDFToEPUB(title, author string, pdfPath string) ([]byte, PDFConverter, error) {
	if data, err := convertWithEbookConvert(pdfPath); err == nil {
		return data, ConverterEbookConvert, nil
	}
	if data, err := convertWithMutool(title, author, pdfPath); err == nil {
		return data, ConverterMutool, nil
	}
	if text, err := convertWithPdftotext(pdfPath); err == nil {
		return buildEPUBFromPlainText(title, author, text), ConverterPdftotext, nil
	}
	text, err := extractPDFTextPureGo(pdfPath)
	if err != nil {
		return nil, ConverterUnavailable, fmt.Errorf("pdf to epub: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, ConverterUnavailable, errors.New("pdf appears to contain no extractable text — install Calibre, mupdf-tools, or poppler-utils for better results, or this is a scanned PDF needing OCR")
	}
	return buildEPUBFromPlainText(title, author, text), ConverterPureGo, nil
}

// ----- Tier 1: Calibre's ebook-convert (best fidelity) -----

func convertWithEbookConvert(pdfPath string) ([]byte, error) {
	bin, err := exec.LookPath("ebook-convert")
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "moco-pdf-cal-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "out.epub")

	// Calibre's ebook-convert pulls in Qt; force headless rendering so it
	// works in containers and on servers without an X display.
	//
	// Flag rationale:
	//   --no-default-epub-cover: skip Calibre's ornamental fallback cover
	//     (the decorated frame with just the title) when it can't detect a
	//     real cover. We'd rather have no cover than a misleading one.
	//   --remove-first-image: PDF importers commonly use page 1 as the
	//     EPUB cover AND include it as the first chapter image — this
	//     dedupes that.
	//   --pretty-print: cleaner XHTML inside the EPUB (better for our reader).
	cmd := exec.Command(bin, pdfPath, outPath,
		"--no-default-epub-cover",
		"--remove-first-image",
		"--pretty-print",
	)
	cmd.Env = append(os.Environ(),
		"QT_QPA_PLATFORM=offscreen",
		"DISPLAY=", // explicitly empty so Calibre doesn't try to use a host display
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ebook-convert: %w (%s)", err, stderr.String())
	}
	return os.ReadFile(outPath)
}

// ----- Tier 2: MuPDF's mutool (text + images, lightweight) -----

// convertWithMutool runs `mutool convert -F xhtml` for text + structure and
// `mutool extract` to recover raster images from the PDF, then packages them
// into a valid EPUB archive. Vector graphics in the PDF are not preserved
// (mutool's text-mode output drops drawing operators) — install Calibre's
// ebook-convert for full image fidelity.
func convertWithMutool(title, author, pdfPath string) ([]byte, error) {
	bin, err := exec.LookPath("mutool")
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "moco-pdf-mu-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// Step 1: text+structure as XHTML.
	outPath := filepath.Join(tmpDir, "book.xhtml")
	cmd := exec.Command(bin, "convert", "-F", "xhtml", "-o", outPath, pdfPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mutool convert: %w (%s)", err, stderr.String())
	}
	xhtml, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}

	// Step 2: explicit raster-image extraction. mutool's xhtml output only
	// emits inline data URIs for embedded raster XObjects when it can decode
	// them; for many real-world PDFs the images aren't there. `mutool
	// extract` runs at the PDF-object level and dumps every image it finds.
	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err == nil {
		extractCmd := exec.Command(bin, "extract", pdfPath)
		extractCmd.Dir = extractDir
		_ = extractCmd.Run() // best-effort: we still build an EPUB even if this fails
	}

	// Collect images from BOTH locations: inline siblings of the xhtml, and
	// the explicit extract dump.
	var images []imgFile
	for _, dir := range []string{tmpDir, extractDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if name == filepath.Base(outPath) {
				continue
			}
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" && ext != ".bmp" {
				continue
			}
			body, rerr := os.ReadFile(filepath.Join(dir, name))
			if rerr != nil || len(body) < 256 {
				continue // skip tiny noise images / read errors
			}
			images = append(images, imgFile{origName: uniqueImageName(name, images), body: body})
		}
	}

	// Rewrite any inline image references inside the XHTML to use
	// `images/<basename>` so they resolve inside the EPUB.
	xhtmlStr := string(xhtml)
	for _, img := range images {
		xhtmlStr = strings.ReplaceAll(xhtmlStr, img.origName, "images/"+img.origName)
	}

	body := extractXHTMLBody(xhtmlStr)
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("mutool produced empty XHTML")
	}

	chapters := splitXHTMLByPages(title, body)

	// If extract recovered images that aren't already referenced in the
	// xhtml, append them as a final "Figures" chapter so the user at least
	// sees them. Position info isn't recoverable cheaply.
	orphanImages := imagesNotReferenced(images, xhtmlStr)
	if len(orphanImages) > 0 {
		chapters = append(chapters, Chapter{
			ID:    "figures",
			Title: "Figures",
			HTML:  wrapXHTMLChapter("Figures", buildFiguresHTML(orphanImages)),
		})
	}

	// Build EPUB.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = writeStoredFile(zw, "mimetype", []byte("application/epub+zip"))
	_ = writeFile(zw, "META-INF/container.xml", []byte(containerXML))
	_ = writeFile(zw, "OEBPS/styles.css", []byte(defaultCSS))

	for idx, ch := range chapters {
		_ = writeFile(zw, fmt.Sprintf("OEBPS/chapter-%02d.xhtml", idx+1), []byte(ch.HTML))
	}
	for _, img := range images {
		_ = writeFile(zw, "OEBPS/images/"+img.origName, img.body)
	}

	_ = writeFile(zw, "OEBPS/content.opf", []byte(buildOPFWithImages(title, author, chapters, images)))
	_ = writeFile(zw, "OEBPS/nav.xhtml", []byte(buildNav(title, chapters)))
	_ = zw.Close()
	return buf.Bytes(), nil
}

// uniqueImageName returns a name that doesn't collide with already-collected images.
func uniqueImageName(name string, existing []imgFile) string {
	taken := map[string]bool{}
	for _, e := range existing {
		taken[e.origName] = true
	}
	if !taken[name] {
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, i, ext)
		if !taken[candidate] {
			return candidate
		}
	}
}

func imagesNotReferenced(images []imgFile, xhtml string) []imgFile {
	var orphans []imgFile
	for _, img := range images {
		if !strings.Contains(xhtml, img.origName) {
			orphans = append(orphans, img)
		}
	}
	return orphans
}

func buildFiguresHTML(images []imgFile) string {
	var sb strings.Builder
	sb.WriteString(`<section><h1>Figures</h1><p>Images recovered from the original PDF:</p>`)
	for _, img := range images {
		sb.WriteString(fmt.Sprintf(
			`<figure style="margin:1.5em 0;text-align:center"><img src="images/%s" alt="%s" style="max-width:100%%;height:auto"/></figure>`,
			html.EscapeString(img.origName),
			html.EscapeString(img.origName),
		))
	}
	sb.WriteString(`</section>`)
	return sb.String()
}

var bodyOpen = regexp.MustCompile(`(?is)<body[^>]*>`)
var bodyClose = regexp.MustCompile(`(?is)</body>`)

// extractXHTMLBody returns just the inner content between <body>...</body>.
func extractXHTMLBody(doc string) string {
	openLoc := bodyOpen.FindStringIndex(doc)
	closeLoc := bodyClose.FindStringIndex(doc)
	if openLoc == nil || closeLoc == nil || closeLoc[0] < openLoc[1] {
		return doc
	}
	return doc[openLoc[1]:closeLoc[0]]
}

// pageDivRE matches mutool's per-page wrapper (`<div id="page1" ...>`). The
// exact attribute order varies across mutool versions, so we match flexibly.
var pageDivRE = regexp.MustCompile(`(?is)<div[^>]*\bid=["']?page\d+["']?[^>]*>`)

// splitXHTMLByPages chunks mutool's body XHTML into chapters of ~10 pages.
// Falls back to one big chapter if no page divs are detected.
func splitXHTMLByPages(title, body string) []Chapter {
	indices := pageDivRE.FindAllStringIndex(body, -1)
	if len(indices) < 2 {
		return []Chapter{{
			ID:    slugID(title, 1),
			Title: title,
			HTML:  wrapXHTMLChapter(title, body),
		}}
	}
	const pagesPerChapter = 10
	var chapters []Chapter
	groupStart := 0
	for i := pagesPerChapter; i < len(indices); i += pagesPerChapter {
		segment := body[indices[groupStart][0]:indices[i][0]]
		chapters = append(chapters, Chapter{
			ID:    slugID(title, len(chapters)+1),
			Title: fmt.Sprintf("Pages %d–%d", groupStart+1, i),
			HTML:  wrapXHTMLChapter(fmt.Sprintf("%s — pages %d–%d", title, groupStart+1, i), segment),
		})
		groupStart = i
	}
	// Trailing pages.
	if groupStart < len(indices) {
		segment := body[indices[groupStart][0]:]
		chapters = append(chapters, Chapter{
			ID:    slugID(title, len(chapters)+1),
			Title: fmt.Sprintf("Pages %d–%d", groupStart+1, len(indices)),
			HTML:  wrapXHTMLChapter(fmt.Sprintf("%s — pages %d–%d", title, groupStart+1, len(indices)), segment),
		})
	}
	return chapters
}

func wrapXHTMLChapter(title, body string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <title>%s</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    %s
  </body>
</html>`, html.EscapeString(title), body)
}

type imgFile struct {
	origName string
	body     []byte
}

// buildOPFWithImages is like buildOPF but also lists image files in the
// manifest so they're recognized by EPUB readers.
func buildOPFWithImages(title, author string, chapters []Chapter, images []imgFile) string {
	manifest := []string{
		`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`,
		`    <item id="css" href="styles.css" media-type="text/css"/>`,
	}
	spine := make([]string, 0, len(chapters))
	for idx := range chapters {
		id := fmt.Sprintf("chap-%02d", idx+1)
		manifest = append(manifest, fmt.Sprintf(
			`    <item id="%s" href="chapter-%02d.xhtml" media-type="application/xhtml+xml"/>`, id, idx+1))
		spine = append(spine, fmt.Sprintf(`    <itemref idref="%s"/>`, id))
	}
	for i, img := range images {
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(img.origName)))
		if mt == "" {
			mt = "image/png"
		}
		manifest = append(manifest, fmt.Sprintf(
			`    <item id="img-%d" href="images/%s" media-type="%s"/>`,
			i+1, html.EscapeString(img.origName), mt))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:moco:%s</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
%s
  </manifest>
  <spine>
%s
  </spine>
</package>`,
		slugID(title, 0),
		html.EscapeString(title),
		html.EscapeString(author),
		strings.Join(manifest, "\n"),
		strings.Join(spine, "\n"))
}

// ----- Tier 3: pdftotext (text only, layout-aware) -----

func convertWithPdftotext(pdfPath string) (string, error) {
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "-layout", "-enc", "UTF-8", pdfPath, "-")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// ----- Tier 4: pure-Go fallback (no images, basic text) -----

func extractPDFTextPureGo(path string) (string, error) {
	f, r, err := pdfReader.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	totalPages := r.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return sb.String(), nil
}

// ----- Plain-text → EPUB packaging (used by tier 3 + 4) -----

func buildEPUBFromPlainText(title, author, text string) []byte {
	chapters := splitTextIntoChapters(title, text)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = writeStoredFile(zw, "mimetype", []byte("application/epub+zip"))
	_ = writeFile(zw, "META-INF/container.xml", []byte(containerXML))
	_ = writeFile(zw, "OEBPS/content.opf", []byte(buildOPF(title, author, chapters)))
	_ = writeFile(zw, "OEBPS/nav.xhtml", []byte(buildNav(title, chapters)))
	_ = writeFile(zw, "OEBPS/styles.css", []byte(defaultCSS))
	for idx, ch := range chapters {
		_ = writeFile(zw, fmt.Sprintf("OEBPS/chapter-%02d.xhtml", idx+1), []byte(ch.HTML))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func splitTextIntoChapters(fallbackTitle, text string) []Chapter {
	lines := strings.Split(text, "\n")
	chapters := []Chapter{}
	currentTitle := fallbackTitle
	var paragraphs []string

	flush := func() {
		if len(paragraphs) == 0 && currentTitle == "" {
			return
		}
		body := paragraphsToHTML(paragraphs)
		chapters = append(chapters, Chapter{
			ID:    slugID(currentTitle, len(chapters)+1),
			Title: currentTitle,
			HTML:  chapterXHTML(currentTitle, body),
		})
		paragraphs = nil
	}

	for _, raw := range lines {
		line := strings.TrimRightFunc(raw, func(r rune) bool { return r == ' ' || r == '\t' })
		if heading := detectHeading(line); heading != "" {
			if len(paragraphs) > 0 || len(chapters) > 0 {
				flush()
			}
			currentTitle = heading
			continue
		}
		paragraphs = append(paragraphs, line)
	}
	flush()

	if len(chapters) == 0 {
		chapters = append(chapters, Chapter{
			ID:    slugID(fallbackTitle, 1),
			Title: fallbackTitle,
			HTML:  chapterXHTML(fallbackTitle, paragraphsToHTML(strings.Split(text, "\n"))),
		})
	}
	return chapters
}

func detectHeading(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "chapter ") && len(trimmed) <= 80 {
		return trimmed
	}
	if strings.HasPrefix(lower, "part ") && len(trimmed) <= 80 {
		return trimmed
	}
	if len(trimmed) <= 60 && trimmed == strings.ToUpper(trimmed) && hasAlpha(trimmed) {
		return trimmed
	}
	return ""
}

func hasAlpha(s string) bool {
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

func paragraphsToHTML(lines []string) string {
	var blocks []string
	var buf []string
	flush := func() {
		if len(buf) == 0 {
			return
		}
		joined := strings.TrimSpace(strings.Join(buf, " "))
		if joined != "" {
			blocks = append(blocks, "<p>"+html.EscapeString(joined)+"</p>")
		}
		buf = nil
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		// Detect inline subheadings inside a chapter and emit <h2> instead
		// of <p>. Without this every heading the PDF had ("Section 3.2",
		// "Introduction", "RESULTS", etc.) renders as plain body text
		// indistinguishable from a paragraph. Chapter-level headings are
		// already handled in splitTextIntoChapters; this only kicks in for
		// the lines that didn't qualify as a chapter break.
		if level := detectSubheadingLevel(line); level > 0 {
			flush()
			tag := fmt.Sprintf("h%d", level)
			blocks = append(blocks, "<"+tag+">"+html.EscapeString(line)+"</"+tag+">")
			continue
		}
		buf = append(buf, line)
	}
	flush()
	return strings.Join(blocks, "\n      ")
}

// detectSubheadingLevel returns 2 or 3 for lines that look like subheadings
// (and 0 otherwise). Heuristic — PDF text extraction loses font metadata so
// we go by structural cues:
//   - Numbered: "3.", "3.1", "3.1.2 ..." → level reflects depth (1=h2, 2=h3)
//   - "Section N", "Part N" prefixes → h2
//   - Short ALL-CAPS line (already partially covered by detectHeading at
//     chapter-level, but anything that survived to here is a sub-section)
//   - Title-case short line ending without sentence punctuation
//
// A line that's already a chapter heading was filtered out upstream, so
// false positives at the chapter level can't sneak in here.
var (
	numberedHeadingRE = regexp.MustCompile(`^(\d+(?:\.\d+){0,3})\.?\s+\S`)
	sectionPrefixRE   = regexp.MustCompile(`(?i)^(section|part|appendix)\s+[a-z0-9]`)
)

func detectSubheadingLevel(line string) int {
	if line == "" || len(line) > 90 {
		return 0
	}
	// Lines that end with a sentence-ending punctuation are paragraphs, not
	// headings — most real headings don't end in `.`, `!`, `?`, `:`, or a
	// quote/paren close.
	last := line[len(line)-1]
	if last == '.' || last == '!' || last == '?' || last == ',' {
		return 0
	}
	if m := numberedHeadingRE.FindStringSubmatch(line); m != nil {
		dots := strings.Count(m[1], ".")
		if dots >= 2 {
			return 3
		}
		return 2
	}
	if sectionPrefixRE.MatchString(line) {
		return 2
	}
	if len(line) <= 60 && line == strings.ToUpper(line) && hasAlpha(line) {
		return 2
	}
	return 0
}

func chapterXHTML(title, body string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
  <head>
    <title>%s</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <section>
      <h1>%s</h1>
      %s
    </section>
  </body>
</html>`, html.EscapeString(title), html.EscapeString(title), body)
}
