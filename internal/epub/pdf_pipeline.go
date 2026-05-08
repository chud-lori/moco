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
	"sort"
	"strings"
	"unicode"

	pdfReader "github.com/ledongthuc/pdf"
)

type PDFProfile string

const (
	ProfileDigital        PDFProfile = "digital"
	ProfileScanned        PDFProfile = "scanned"
	ProfileScannedWithOCR PDFProfile = "scanned_with_ocr"
	ProfileMixed          PDFProfile = "mixed"
)

type PDFAnalysis struct {
	Profile              PDFProfile
	Pages                int
	DigitalPages         int
	ScannedPages         int
	ScannedOCRPages      int
	MixedPages           int
	AverageTextChars     float64
	AverageImagesPerPage float64
	MathHeavyPages       int
	LooksMathHeavy       bool
	PageDetails          []PDFPageAnalysis
}

type PDFPageAnalysis struct {
	Index      int
	Profile    PDFProfile
	Text       string
	ImageCount int
	MathHeavy  bool
}

type pageProfile int

const (
	pageDigital pageProfile = iota
	pageScanned
	pageScannedWithOCR
	pageMixed
)

func AnalyzePDF(path string) (PDFAnalysis, error) {
	f, r, err := pdfReader.Open(path)
	if err != nil {
		return PDFAnalysis{}, err
	}
	defer f.Close()

	var analysis PDFAnalysis
	analysis.Pages = r.NumPage()
	if analysis.Pages == 0 {
		analysis.Profile = ProfileScanned
		return analysis, nil
	}

	var totalChars int
	var totalImages int
	for i := 1; i <= analysis.Pages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, _ := page.GetPlainText(nil)
		text = strings.TrimSpace(text)
		textChars := len([]rune(text))
		imageCount := countPageImages(page)
		mathHeavy := looksMathHeavyText(text)

		totalChars += textChars
		totalImages += imageCount

		pageType := classifyPage(text, imageCount)
		pageAnalysis := PDFPageAnalysis{
			Index:      i,
			Profile:    pageProfileLabel(pageType),
			Text:       text,
			ImageCount: imageCount,
			MathHeavy:  mathHeavy,
		}
		analysis.PageDetails = append(analysis.PageDetails, pageAnalysis)
		if mathHeavy {
			analysis.MathHeavyPages++
		}

		switch pageType {
		case pageScanned:
			analysis.ScannedPages++
		case pageScannedWithOCR:
			analysis.ScannedOCRPages++
		case pageMixed:
			analysis.MixedPages++
		default:
			analysis.DigitalPages++
		}
	}

	analysis.AverageTextChars = float64(totalChars) / float64(analysis.Pages)
	analysis.AverageImagesPerPage = float64(totalImages) / float64(analysis.Pages)
	analysis.LooksMathHeavy = analysis.MathHeavyPages >= maxInt(1, analysis.Pages/3)
	analysis.Profile = summarizePDFProfile(analysis)
	return analysis, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pageProfileLabel(profile pageProfile) PDFProfile {
	switch profile {
	case pageScanned:
		return ProfileScanned
	case pageScannedWithOCR:
		return ProfileScannedWithOCR
	case pageMixed:
		return ProfileMixed
	default:
		return ProfileDigital
	}
}

func classifyPage(text string, imageCount int) pageProfile {
	textChars := len([]rune(text))
	wordCount := len(strings.Fields(text))
	garbageRatio := estimateGarbageRatio(text)

	switch {
	case wordCount < 10 && textChars < 80 && imageCount > 0:
		return pageScanned
	case wordCount < 4 && textChars < 32:
		return pageScanned
	case imageCount > 0 && wordCount >= 25 && garbageRatio >= 0.18:
		return pageScannedWithOCR
	case imageCount > 0 && wordCount >= 40:
		return pageMixed
	case garbageRatio >= 0.25 && imageCount > 0:
		return pageScannedWithOCR
	default:
		return pageDigital
	}
}

func summarizePDFProfile(analysis PDFAnalysis) PDFProfile {
	pages := analysis.Pages
	if pages == 0 {
		return ProfileScanned
	}
	if analysis.ScannedPages+analysis.ScannedOCRPages >= int(float64(pages)*0.8) {
		if analysis.ScannedOCRPages > analysis.ScannedPages {
			return ProfileScannedWithOCR
		}
		return ProfileScanned
	}
	if analysis.MixedPages >= int(float64(pages)*0.25) || (analysis.ScannedPages+analysis.ScannedOCRPages > 0 && analysis.DigitalPages > 0) {
		return ProfileMixed
	}
	if analysis.ScannedOCRPages > 0 {
		return ProfileScannedWithOCR
	}
	return ProfileDigital
}

func countPageImages(page pdfReader.Page) int {
	xObjects := page.Resources().Key("XObject")
	if xObjects.IsNull() {
		return 0
	}
	count := 0
	for _, key := range xObjects.Keys() {
		if xObjects.Key(key).Key("Subtype").Name() == "Image" {
			count++
		}
	}
	return count
}

func estimateGarbageRatio(text string) float64 {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 1
	}
	var suspicious int
	for _, r := range runes {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsSpace(r):
		case strings.ContainsRune(".,;:!?\"'()[]{}-_/&%$#@+=*", r):
		default:
			suspicious++
		}
	}
	return float64(suspicious) / float64(len(runes))
}

func looksMathHeavyText(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return false
	}
	var mathish int
	for _, r := range runes {
		if strings.ContainsRune("=+-*/∑∫√≈≠≤≥∞πλµ∂∆∇", r) {
			mathish++
			continue
		}
		if unicode.IsDigit(r) {
			mathish++
		}
	}
	ratio := float64(mathish) / float64(len(runes))
	return ratio >= 0.18 || strings.Contains(text, "\\frac") || strings.Contains(text, "\\sum")
}

func maybeOCRNormalizePDF(path string, analysis PDFAnalysis) (string, func(), error) {
	if analysis.Profile == ProfileDigital {
		return path, func() {}, nil
	}
	bin, err := exec.LookPath("ocrmypdf")
	if err != nil {
		return path, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "moco-pdf-ocr-")
	if err != nil {
		return path, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	outPath := filepath.Join(tmpDir, "normalized.pdf")

	args := []string{"--deskew", "--clean", "--clean-final", "--optimize", "0"}
	switch analysis.Profile {
	case ProfileScanned:
		args = append(args, "--force-ocr")
	case ProfileScannedWithOCR:
		args = append(args, "--redo-ocr")
	case ProfileMixed:
		args = append(args, "--skip-text")
	}
	args = append(args, path, outPath)

	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return path, func() {}, nil
	}
	if _, err := os.Stat(outPath); err != nil {
		cleanup()
		return path, func() {}, nil
	}
	return outPath, cleanup, nil
}

func convertWithStructuredMarkdown(title, author, pdfPath string, analysis PDFAnalysis) ([]byte, structuredMarkdownResult, error) {
	result, err := selectStructuredMarkdown(pdfPath, analysis)
	if err != nil {
		return nil, structuredMarkdownResult{}, err
	}
	if !structuredMarkdownLooksUsable(result.Markdown) {
		return nil, structuredMarkdownResult{}, fmt.Errorf("structured markdown output was too weak")
	}
	data, err := MarkdownToEPUB(title, author, []byte(result.Markdown))
	return data, result, err
}

type structuredMarkdownResult struct {
	Engine   string
	Markdown string
	Score    float64
}

type structuredExtractor struct {
	name string
	run  func(string) (string, error)
}

func selectStructuredMarkdown(pdfPath string, analysis PDFAnalysis) (structuredMarkdownResult, error) {
	extractors := orderedStructuredExtractors(analysis)

	var best structuredMarkdownResult
	var failures []string
	for _, extractor := range extractors {
		md, err := extractor.run(pdfPath)
		if err != nil {
			failures = append(failures, extractor.name+": "+err.Error())
			continue
		}
		score := scoreStructuredMarkdown(md)
		if score > best.Score {
			best = structuredMarkdownResult{
				Engine:   extractor.name,
				Markdown: md,
				Score:    score,
			}
		}
	}
	if strings.TrimSpace(best.Markdown) == "" {
		if len(failures) == 0 {
			return structuredMarkdownResult{}, fmt.Errorf("no structured extractor available")
		}
		return structuredMarkdownResult{}, errors.New(strings.Join(failures, "; "))
	}
	return best, nil
}

func orderedStructuredExtractors(analysis PDFAnalysis) []structuredExtractor {
	extractors := []structuredExtractor{}
	if analysis.LooksMathHeavy {
		extractors = append(extractors, structuredExtractor{name: "nougat", run: extractStructuredMarkdownNougat})
	}
	extractors = append(extractors,
		structuredExtractor{name: "pymupdf4llm", run: extractStructuredMarkdownPyMuPDF},
		structuredExtractor{name: "docling", run: extractStructuredMarkdownDocling},
		structuredExtractor{name: "marker", run: extractStructuredMarkdownMarker},
	)
	return extractors
}

func extractStructuredMarkdownPyMuPDF(pdfPath string) (string, error) {
	python, err := detectPythonBinary()
	if err != nil {
		return "", err
	}
	scriptPath, cleanup, err := writeEmbeddedPythonScript("moco-pymupdf-", "pymupdf4llm_extract.py")
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.Command(python, scriptPath, pdfPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pymupdf4llm extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func extractStructuredMarkdownDocling(pdfPath string) (string, error) {
	python, err := detectPythonBinary()
	if err != nil {
		return "", err
	}
	scriptPath, cleanup, err := writeEmbeddedPythonScript("moco-docling-", "docling_extract.py")
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.Command(python, scriptPath, pdfPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docling extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func extractStructuredMarkdownMarker(pdfPath string) (string, error) {
	if bin, err := exec.LookPath("marker_single"); err == nil {
		tmpDir, err := os.MkdirTemp("", "moco-marker-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		cmd := exec.Command(bin, pdfPath, "--output_dir", tmpDir, "--output_format", "markdown")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("marker extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return readFirstMarkdownFile(tmpDir)
	}

	python, err := detectPythonBinary()
	if err != nil {
		return "", err
	}
	scriptPath, cleanup, err := writeEmbeddedPythonScript("moco-marker-py-", "marker_extract.py")
	if err != nil {
		return "", err
	}
	defer cleanup()

	cmd := exec.Command(python, scriptPath, pdfPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("marker python extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func extractStructuredMarkdownNougat(pdfPath string) (string, error) {
	if bin, err := exec.LookPath("nougat"); err == nil {
		tmpDir, err := os.MkdirTemp("", "moco-nougat-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		cmd := exec.Command(bin, pdfPath, "-o", tmpDir)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("nougat extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return readFirstMarkdownFile(tmpDir)
	}
	python, err := detectPythonBinary()
	if err != nil {
		return "", err
	}
	scriptPath, cleanup, err := writeEmbeddedPythonScript("moco-nougat-py-", "nougat_extract.py")
	if err != nil {
		return "", err
	}
	defer cleanup()
	cmd := exec.Command(python, scriptPath, pdfPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nougat python extraction failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return "", fmt.Errorf("nougat extraction produced no output")
}

func detectPythonBinary() (string, error) {
	for _, candidate := range []string{"python3", "python"} {
		if bin, err := exec.LookPath(candidate); err == nil {
			return bin, nil
		}
	}
	return "", fmt.Errorf("python not available")
}

func structuredMarkdownLooksUsable(md string) bool {
	trimmed := strings.TrimSpace(md)
	if trimmed == "" {
		return false
	}
	if len(strings.Fields(trimmed)) < 80 {
		return false
	}
	if estimateGarbageRatio(trimmed) > 0.28 {
		return false
	}
	return true
}

func scoreStructuredMarkdown(md string) float64 {
	trimmed := strings.TrimSpace(md)
	if trimmed == "" {
		return 0
	}
	words := len(strings.Fields(trimmed))
	if words == 0 {
		return 0
	}

	headings := strings.Count(trimmed, "\n#")
	paragraphs := strings.Count(trimmed, "\n\n")
	lists := strings.Count(trimmed, "\n- ") + strings.Count(trimmed, "\n1. ")
	tables := strings.Count(trimmed, "\n|")
	codeBlocks := strings.Count(trimmed, "```")
	garbagePenalty := estimateGarbageRatio(trimmed) * 120
	shortLinePenalty := shortLineRatio(trimmed) * 20

	score := float64(words)/20 +
		float64(headings*6) +
		float64(paragraphs*1) +
		float64(lists*2) +
		float64(tables*4) +
		float64(codeBlocks*3) -
		garbagePenalty -
		shortLinePenalty

	if strings.Contains(trimmed, "�") {
		score -= 40
	}
	return score
}

func shortLineRatio(text string) float64 {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return 0
	}
	var short int
	var total int
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		total++
		if len([]rune(trimmed)) < 5 {
			short++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(short) / float64(total)
}

func readFirstMarkdownFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".md" || ext == ".markdown" {
			matches = append(matches, filepath.Join(dir, name))
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", fmt.Errorf("no markdown output found")
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildHybridScannedEPUB(title, author, pdfPath string, analysis PDFAnalysis) ([]byte, error) {
	images, cleanup, err := renderPDFPagesToImages(pdfPath)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if len(images) == 0 {
		return nil, fmt.Errorf("no rendered page images available")
	}

	var chapters []Chapter
	const pagesPerChapter = 8
	for start := 0; start < len(images); start += pagesPerChapter {
		end := start + pagesPerChapter
		if end > len(images) {
			end = len(images)
		}
		titleText := fmt.Sprintf("Pages %d-%d", start+1, end)
		var body strings.Builder
		body.WriteString("    <section class=\"page-gallery\">\n")
		for i := start; i < end; i++ {
			page := lookupPageAnalysis(analysis, i+1)
			body.WriteString(renderHybridPage(images[i], page))
		}
		body.WriteString("    </section>\n")
		chapters = append(chapters, Chapter{
			ID:    slugID(title, len(chapters)+1),
			Title: titleText,
			HTML:  wrapXHTMLChapter(titleText, body.String()),
		})
	}

	return buildEPUBArchive(title, author, chapters, images), nil
}

func renderHybridPage(img imgFile, page PDFPageAnalysis) string {
	if pageCanUseText(page) {
		return renderTextPage(page)
	}
	return renderImageFallbackPage(img, page)
}

func pageCanUseText(page PDFPageAnalysis) bool {
	text := strings.TrimSpace(page.Text)
	if len(strings.Fields(text)) < 30 {
		return false
	}
	if estimateGarbageRatio(text) > 0.22 {
		return false
	}
	return page.Profile == ProfileDigital || page.Profile == ProfileMixed || page.Profile == ProfileScannedWithOCR
}

func renderTextPage(page PDFPageAnalysis) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("      <section class=\"page-text\" id=\"page-%d\">\n", page.Index))
	sb.WriteString(fmt.Sprintf("        <h2>Page %d</h2>\n", page.Index))
	for _, para := range splitPageTextIntoParagraphs(page.Text) {
		sb.WriteString(fmt.Sprintf("        <p>%s</p>\n", html.EscapeString(para)))
	}
	sb.WriteString("      </section>\n")
	return sb.String()
}

func splitPageTextIntoParagraphs(text string) []string {
	var paragraphs []string
	for _, block := range strings.Split(text, "\n\n") {
		normalized := normalizeWhitespace(block)
		if normalized == "" {
			continue
		}
		paragraphs = append(paragraphs, normalized)
	}
	if len(paragraphs) == 0 {
		if normalized := normalizeWhitespace(text); normalized != "" {
			paragraphs = append(paragraphs, normalized)
		}
	}
	return paragraphs
}

func renderImageFallbackPage(img imgFile, page PDFPageAnalysis) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"      <figure class=\"page-scan\" id=\"page-%d\"><img src=\"images/%s\" alt=\"Page %d\" class=\"page-image\" />",
		page.Index,
		html.EscapeString(img.origName),
		page.Index,
	))
	if text := strings.TrimSpace(page.Text); text != "" {
		sb.WriteString(fmt.Sprintf(
			"<figcaption><p class=\"ocr-label\">Recognized text</p><p class=\"ocr-text\">%s</p></figcaption>",
			html.EscapeString(normalizeWhitespace(text)),
		))
	}
	sb.WriteString("</figure>\n")
	return sb.String()
}

func lookupPageAnalysis(analysis PDFAnalysis, index int) PDFPageAnalysis {
	for _, page := range analysis.PageDetails {
		if page.Index == index {
			return page
		}
	}
	return PDFPageAnalysis{Index: index}
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func renderPDFPagesToImages(pdfPath string) ([]imgFile, func(), error) {
	if bin, err := exec.LookPath("mutool"); err == nil {
		return renderPDFPagesToImagesWithMutool(bin, pdfPath)
	}
	if bin, err := exec.LookPath("pdftoppm"); err == nil {
		return renderPDFPagesToImagesWithPdftoppm(bin, pdfPath)
	}
	return nil, func() {}, fmt.Errorf("no page renderer available")
}

func renderPDFPagesToImagesWithMutool(bin, pdfPath string) ([]imgFile, func(), error) {
	tmpDir, err := os.MkdirTemp("", "moco-pdf-pages-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	outputPattern := filepath.Join(tmpDir, "page-%04d.png")
	cmd := exec.Command(bin, "draw", "-F", "png", "-o", outputPattern, pdfPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("mutool draw: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	images, err := collectRenderedImages(tmpDir)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return images, cleanup, nil
}

func renderPDFPagesToImagesWithPdftoppm(bin, pdfPath string) ([]imgFile, func(), error) {
	tmpDir, err := os.MkdirTemp("", "moco-pdf-pages-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	prefix := filepath.Join(tmpDir, "page")
	cmd := exec.Command(bin, "-png", pdfPath, prefix)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("pdftoppm: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	images, err := collectRenderedImages(tmpDir)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return images, cleanup, nil
}

func collectRenderedImages(dir string) ([]imgFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var images []imgFile
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		images = append(images, imgFile{origName: name, body: body})
	}
	return images, nil
}

func buildEPUBArchive(title, author string, chapters []Chapter, images []imgFile) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = writeStoredFile(zw, "mimetype", []byte("application/epub+zip"))
	_ = writeFile(zw, "META-INF/container.xml", []byte(containerXML))
	_ = writeFile(zw, "OEBPS/content.opf", []byte(buildOPFWithImages(title, author, chapters, images)))
	_ = writeFile(zw, "OEBPS/nav.xhtml", []byte(buildNav(title, chapters)))
	_ = writeFile(zw, "OEBPS/styles.css", []byte(defaultCSS))
	for idx, ch := range chapters {
		_ = writeFile(zw, fmt.Sprintf("OEBPS/chapter-%02d.xhtml", idx+1), []byte(ch.HTML))
	}
	for _, img := range images {
		_ = writeFile(zw, "OEBPS/images/"+img.origName, img.body)
	}
	_ = zw.Close()
	return buf.Bytes()
}
