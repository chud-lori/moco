package epub

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pdfReader "github.com/ledongthuc/pdf"
)

// PDFToEPUB converts a PDF on disk to an EPUB byte slice. It prefers external
// converters (Calibre's ebook-convert, then poppler's pdftotext) for higher
// fidelity output and falls back to pure-Go text extraction when neither is
// installed. The resulting EPUB is always a valid EPUB 3 archive.
func PDFToEPUB(title, author string, pdfPath string) ([]byte, error) {
	if data, err := convertWithEbookConvert(pdfPath); err == nil {
		return data, nil
	}
	if text, err := convertWithPdftotext(pdfPath); err == nil {
		return buildEPUBFromPlainText(title, author, text), nil
	}
	text, err := extractPDFTextPureGo(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("pdf to epub: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("pdf appears to contain no extractable text (scanned PDFs need OCR)")
	}
	return buildEPUBFromPlainText(title, author, text), nil
}

// convertWithEbookConvert shells out to Calibre's ebook-convert when present.
// This produces the best PDF→EPUB output but requires Calibre installed.
func convertWithEbookConvert(pdfPath string) ([]byte, error) {
	bin, err := exec.LookPath("ebook-convert")
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "moco-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	outPath := filepath.Join(tmpDir, "out.epub")
	cmd := exec.Command(bin, pdfPath, outPath)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(outPath)
}

// convertWithPdftotext uses poppler's pdftotext to extract layout-aware text,
// which gives much better paragraph reconstruction than the pure-Go path.
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

// buildEPUBFromPlainText splits a plain text dump into paragraphs (and
// best-effort chapter sections from "Chapter N" headings or all-caps lines)
// and assembles a valid EPUB 3 archive.
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
		doc := chapterXHTML(currentTitle, body)
		chapters = append(chapters, Chapter{
			ID:    slugID(currentTitle, len(chapters)+1),
			Title: currentTitle,
			HTML:  doc,
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

// detectHeading recognizes obvious chapter markers ("Chapter 1", "CHAPTER I",
// or all-uppercase short lines). Returns the heading text or "" if not.
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
	// Short ALL-CAPS line: likely a section heading.
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
	var paragraphs []string
	var buf []string
	flush := func() {
		if len(buf) == 0 {
			return
		}
		joined := strings.TrimSpace(strings.Join(buf, " "))
		if joined != "" {
			paragraphs = append(paragraphs, "<p>"+html.EscapeString(joined)+"</p>")
		}
		buf = nil
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}
		buf = append(buf, line)
	}
	flush()
	return strings.Join(paragraphs, "\n      ")
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

