package epub

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

type Chapter struct {
	ID    string
	Title string
	HTML  string
}

var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
).Parser()

func MarkdownToEPUB(title, author string, src []byte) ([]byte, error) {
	body, fmTitle, fmAuthor := stripFrontMatter(string(src))
	if title == "" && fmTitle != "" {
		title = fmTitle
	}
	if author == "" && fmAuthor != "" {
		author = fmAuthor
	}
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}

	chapters := parseMarkdown(title, body)

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	if err := writeStoredFile(zipWriter, "mimetype", []byte("application/epub+zip")); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "META-INF/container.xml", []byte(containerXML)); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "OEBPS/content.opf", []byte(buildOPF(title, author, chapters))); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "OEBPS/nav.xhtml", []byte(buildNav(title, chapters))); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "OEBPS/styles.css", []byte(defaultCSS)); err != nil {
		return nil, err
	}

	for idx, chapter := range chapters {
		path := fmt.Sprintf("OEBPS/chapter-%02d.xhtml", idx+1)
		if err := writeFile(zipWriter, path, []byte(chapter.HTML)); err != nil {
			return nil, err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// stripFrontMatter peels off a leading `--- ... ---` YAML block, returning the
// remaining markdown body and the title/author keys if present.
func stripFrontMatter(src string) (body, title, author string) {
	m := yamlBlockRE.FindStringSubmatchIndex(src)
	if m == nil {
		return src, "", ""
	}
	title, author = readYAMLFrontMatter(src[:m[1]])
	return strings.TrimLeft(src[m[1]:], "\r\n"), title, author
}

type markdownSection struct {
	title string
	nodes []gast.Node
}

func parseMarkdown(fallbackTitle, markdown string) []Chapter {
	source := []byte(markdown)
	doc := markdownParser.Parse(text.NewReader(source))

	var sections []markdownSection
	current := markdownSection{title: fallbackTitle}

	flush := func() {
		if strings.TrimSpace(current.title) == "" && len(current.nodes) == 0 {
			return
		}
		sections = append(sections, current)
		current = markdownSection{title: fallbackTitle}
	}

	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		if heading, ok := node.(*gast.Heading); ok && heading.Level == 1 {
			if len(current.nodes) > 0 || len(sections) > 0 {
				flush()
			}
			current.title = strings.TrimSpace(extractInlineText(heading, source))
			if current.title == "" {
				current.title = fallbackTitle
			}
			continue
		}
		current.nodes = append(current.nodes, node)
	}
	flush()

	if len(sections) == 0 {
		sections = append(sections, markdownSection{title: fallbackTitle})
	}

	chapters := make([]Chapter, 0, len(sections))
	for idx, section := range sections {
		title := strings.TrimSpace(section.title)
		if title == "" {
			title = fallbackTitle
		}
		body := renderBlockList(section.nodes, source)
		chapters = append(chapters, Chapter{
			ID:    slugID(title, idx+1),
			Title: title,
			HTML:  chapterXHTML(title, body),
		})
	}
	return chapters
}

func renderBlockList(nodes []gast.Node, source []byte) string {
	var sb strings.Builder
	for _, node := range nodes {
		sb.WriteString(renderBlock(node, source))
	}
	return sb.String()
}

func renderBlock(node gast.Node, source []byte) string {
	switch n := node.(type) {
	case *gast.Paragraph:
		return fmt.Sprintf("      <p>%s</p>\n", renderInlineChildren(n, source))
	case *gast.Heading:
		level := n.Level
		if level < 2 {
			level = 2
		}
		if level > 6 {
			level = 6
		}
		return fmt.Sprintf("      <h%d>%s</h%d>\n", level, renderInlineChildren(n, source), level)
	case *gast.Blockquote:
		return fmt.Sprintf("      <blockquote>\n%s      </blockquote>\n", indentBlock(renderChildBlocks(n, source), 2))
	case *gast.List:
		tag := "ul"
		attrs := ""
		if n.IsOrdered() {
			tag = "ol"
			if n.Start > 1 {
				attrs = fmt.Sprintf(` start="%d"`, n.Start)
			}
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("      <%s%s>\n", tag, attrs))
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			sb.WriteString(renderBlock(child, source))
		}
		sb.WriteString(fmt.Sprintf("      </%s>\n", tag))
		return sb.String()
	case *gast.ListItem:
		return fmt.Sprintf("        <li>%s</li>\n", renderListItemContent(n, source))
	case *gast.FencedCodeBlock:
		return renderCodeBlock(n.Language(source), n.Lines(), source)
	case *gast.CodeBlock:
		return renderCodeBlock(nil, n.Lines(), source)
	case *gast.ThematicBreak:
		return "      <hr />\n"
	case *gast.HTMLBlock:
		return fmt.Sprintf("      <pre><code>%s</code></pre>\n", html.EscapeString(string(n.Text(source))))
	case *gast.TextBlock:
		return fmt.Sprintf("      <p>%s</p>\n", html.EscapeString(strings.TrimSpace(string(n.Text(source)))))
	case *extast.Table:
		return renderTable(n, source)
	case *extast.TableHeader:
		return ""
	case *extast.TableRow:
		return ""
	case *extast.TableCell:
		return ""
	default:
		if node.HasChildren() {
			return renderChildBlocks(node, source)
		}
		return ""
	}
}

func renderChildBlocks(node gast.Node, source []byte) string {
	var children []gast.Node
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		children = append(children, child)
	}
	return renderBlockList(children, source)
}

func renderListItemContent(node *gast.ListItem, source []byte) string {
	var parts []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		rendered := strings.TrimSpace(renderBlock(child, source))
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return "\n" + indentBlock(strings.Join(parts, "\n")+"\n", 3) + "        "
	}
}

func renderTable(node *extast.Table, source []byte) string {
	var sb strings.Builder
	sb.WriteString("      <table>\n")
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch rowGroup := child.(type) {
		case *extast.TableHeader:
			sb.WriteString("        <thead>\n")
			for row := rowGroup.FirstChild(); row != nil; row = row.NextSibling() {
				sb.WriteString(renderTableRow(row, source, true))
			}
			sb.WriteString("        </thead>\n")
		case *extast.TableRow:
			sb.WriteString("        <tbody>\n")
			for row := child; row != nil; row = row.NextSibling() {
				if tableRow, ok := row.(*extast.TableRow); ok {
					sb.WriteString(renderTableRow(tableRow, source, false))
				}
			}
			sb.WriteString("        </tbody>\n")
			return sb.String() + "      </table>\n"
		}
	}
	sb.WriteString("      </table>\n")
	return sb.String()
}

func renderTableRow(node gast.Node, source []byte, header bool) string {
	tag := "td"
	if header {
		tag = "th"
	}
	var sb strings.Builder
	sb.WriteString("          <tr>\n")
	for cell := node.FirstChild(); cell != nil; cell = cell.NextSibling() {
		tableCell, ok := cell.(*extast.TableCell)
		if !ok {
			continue
		}
		alignAttr := ""
		if tableCell.Alignment != extast.AlignNone {
			alignAttr = fmt.Sprintf(` style="text-align:%s"`, tableCell.Alignment.String())
		}
		sb.WriteString(fmt.Sprintf("            <%s%s>%s</%s>\n", tag, alignAttr, renderInlineChildren(tableCell, source), tag))
	}
	sb.WriteString("          </tr>\n")
	return sb.String()
}

func renderCodeBlock(language []byte, lines *text.Segments, source []byte) string {
	langClass := ""
	if len(language) > 0 {
		langClass = fmt.Sprintf(` class="language-%s"`, html.EscapeString(string(language)))
	}
	return fmt.Sprintf("      <pre><code%s>%s</code></pre>\n", langClass, html.EscapeString(renderSegments(lines, source)))
}

func renderSegments(lines *text.Segments, source []byte) string {
	var sb strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		sb.Write(segment.Value(source))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderInlineChildren(node gast.Node, source []byte) string {
	var sb strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		sb.WriteString(renderInline(child, source))
	}
	return sb.String()
}

func renderInline(node gast.Node, source []byte) string {
	switch n := node.(type) {
	case *gast.Text:
		text := html.EscapeString(string(n.Value(source)))
		if n.HardLineBreak() {
			return text + "<br />"
		}
		if n.SoftLineBreak() {
			return text + " "
		}
		return text
	case *gast.String:
		return html.EscapeString(string(n.Value))
	case *gast.CodeSpan:
		return fmt.Sprintf("<code>%s</code>", html.EscapeString(extractInlineText(n, source)))
	case *gast.Emphasis:
		tag := "em"
		if n.Level >= 2 {
			tag = "strong"
		}
		return fmt.Sprintf("<%s>%s</%s>", tag, renderInlineChildren(n, source), tag)
	case *gast.Link:
		href := strings.TrimSpace(string(n.Destination))
		label := renderInlineChildren(n, source)
		if !safeLinkURL(href) {
			return label
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(href), label)
	case *gast.Image:
		src := strings.TrimSpace(string(n.Destination))
		alt := extractInlineText(n, source)
		if !safeLinkURL(src) {
			return html.EscapeString(alt)
		}
		return fmt.Sprintf(`<img src="%s" alt="%s" />`, html.EscapeString(src), html.EscapeString(alt))
	case *gast.AutoLink:
		href := strings.TrimSpace(string(n.URL(source)))
		label := html.EscapeString(string(n.Label(source)))
		if !safeLinkURL(href) {
			return label
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(href), label)
	case *gast.RawHTML:
		return html.EscapeString(string(n.Text(source)))
	case *extast.Strikethrough:
		return fmt.Sprintf("<del>%s</del>", renderInlineChildren(n, source))
	default:
		if node.HasChildren() {
			return renderInlineChildren(node, source)
		}
		return ""
	}
}

func extractInlineText(node gast.Node, source []byte) string {
	var sb strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *gast.Text:
			sb.Write(n.Value(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				sb.WriteByte(' ')
			}
		case *gast.String:
			sb.Write(n.Value)
		case *gast.AutoLink:
			sb.Write(n.Label(source))
		default:
			sb.WriteString(extractInlineText(child, source))
		}
	}
	return strings.TrimSpace(sb.String())
}

func safeLinkURL(u string) bool {
	lower := strings.ToLower(strings.TrimSpace(u))
	if lower == "" {
		return false
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "/") ||
		strings.HasPrefix(lower, "#") {
		return true
	}
	if i := strings.Index(lower, ":"); i > 0 && i < strings.Index(lower+"/", "/") {
		return false
	}
	return true
}

func buildOPF(title, author string, chapters []Chapter) string {
	manifest := []string{
		`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>`,
		`    <item id="css" href="styles.css" media-type="text/css"/>`,
	}
	spine := make([]string, 0, len(chapters))

	for idx := range chapters {
		id := fmt.Sprintf("chap-%02d", idx+1)
		href := fmt.Sprintf("chapter-%02d.xhtml", idx+1)
		manifest = append(manifest, fmt.Sprintf(`    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`, id, href))
		spine = append(spine, fmt.Sprintf(`    <itemref idref="%s"/>`, id))
	}

	bookID := sha1.Sum([]byte(title + author + time.Now().UTC().Format(time.RFC3339Nano)))

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
		hex.EncodeToString(bookID[:]),
		html.EscapeString(title),
		html.EscapeString(author),
		strings.Join(manifest, "\n"),
		strings.Join(spine, "\n"),
	)
}

func buildNav(title string, chapters []Chapter) string {
	items := make([]string, 0, len(chapters))
	for idx, chapter := range chapters {
		items = append(items, fmt.Sprintf(
			`        <li><a href="chapter-%02d.xhtml">%s</a></li>`,
			idx+1,
			html.EscapeString(chapter.Title),
		))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>%s</title>
  </head>
  <body>
    <nav epub:type="toc" id="toc">
      <h1>Contents</h1>
      <ol>
%s
      </ol>
    </nav>
  </body>
</html>`, html.EscapeString(title), strings.Join(items, "\n"))
}

func slugID(title string, index int) string {
	normalized := strings.ToLower(strings.TrimSpace(title))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	if normalized == "" {
		normalized = "chapter"
	}
	return fmt.Sprintf("%s-%d", normalized, index)
}

func writeStoredFile(zw *zip.Writer, name string, body []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, bytes.NewReader(body))
	return err
}

func writeFile(zw *zip.Writer, name string, body []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, bytes.NewReader(body))
	return err
}

func indentBlock(src string, levels int) string {
	prefix := strings.Repeat("  ", levels)
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}

const containerXML = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

const defaultCSS = `body {
  font-family: Georgia, serif;
  line-height: 1.6;
  margin: 5%;
}

h1, h2, h3, h4, h5, h6 {
  font-family: "Palatino Linotype", serif;
}

p,
blockquote,
pre,
ul,
ol,
table {
  margin: 0 0 1em;
}

blockquote {
  border-left: 0.2rem solid #c8b89d;
  margin-left: 0;
  padding-left: 1rem;
}

pre {
  background: #f5f1ea;
  overflow-x: auto;
  padding: 0.9rem;
}

code {
  font-family: monospace;
}

table {
  border-collapse: collapse;
  width: 100%;
}

th,
td {
  border: 1px solid #d9cdb8;
  padding: 0.5rem;
  vertical-align: top;
}

img {
  height: auto;
  max-width: 100%;
}

.page-gallery {
  display: block;
}

.page-scan {
  margin: 0 0 2rem;
  page-break-inside: avoid;
}

.page-image {
  border: 1px solid #d9cdb8;
  display: block;
  width: 100%;
}

.ocr-label {
  font-size: 0.85rem;
  font-weight: bold;
  letter-spacing: 0.04em;
  margin: 0.9rem 0 0.35rem;
  text-transform: uppercase;
}

.ocr-text {
  color: #4f473d;
  font-size: 0.95rem;
}`
