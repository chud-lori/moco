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
)

type Chapter struct {
	ID    string
	Title string
	HTML  string
}

func MarkdownToEPUB(title, author string, src []byte) ([]byte, error) {
	chapters := parseMarkdown(title, string(src))

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	if err := writeStoredFile(zipWriter, "mimetype", []byte("application/epub+zip")); err != nil {
		return nil, err
	}

	if err := writeFile(zipWriter, "META-INF/container.xml", []byte(containerXML)); err != nil {
		return nil, err
	}

	opf := buildOPF(title, author, chapters)
	nav := buildNav(title, chapters)

	if err := writeFile(zipWriter, "OEBPS/content.opf", []byte(opf)); err != nil {
		return nil, err
	}
	if err := writeFile(zipWriter, "OEBPS/nav.xhtml", []byte(nav)); err != nil {
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

func parseMarkdown(fallbackTitle, markdown string) []Chapter {
	lines := strings.Split(markdown, "\n")
	chapters := []Chapter{}

	currentTitle := fallbackTitle
	paragraphs := []string{}

	flush := func() {
		if currentTitle == "" && len(paragraphs) == 0 {
			return
		}
		htmlBody := renderParagraphs(paragraphs)
		chapterID := slugID(currentTitle, len(chapters)+1)
		doc := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
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
</html>`, html.EscapeString(currentTitle), html.EscapeString(currentTitle), htmlBody)
		chapters = append(chapters, Chapter{
			ID:    chapterID,
			Title: currentTitle,
			HTML:  doc,
		})
		paragraphs = nil
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "# ") {
			if len(paragraphs) > 0 || len(chapters) > 0 {
				flush()
			}
			currentTitle = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		if strings.HasPrefix(line, "## ") && len(paragraphs) > 0 {
			flush()
			currentTitle = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		paragraphs = append(paragraphs, raw)
	}

	flush()

	if len(chapters) == 0 {
		chapters = append(chapters, Chapter{
			ID:    slugID(fallbackTitle, 1),
			Title: fallbackTitle,
			HTML:  renderSingleDocument(fallbackTitle, markdown),
		})
	}

	return chapters
}

func renderParagraphs(lines []string) string {
	var paragraphs []string
	var block []string

	flush := func() {
		if len(block) == 0 {
			return
		}
		text := strings.Join(block, " ")
		paragraphs = append(paragraphs, "<p>"+inlineMarkdown(text)+"</p>")
		block = nil
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			continue
		}

		if strings.HasPrefix(line, "### ") {
			flush()
			paragraphs = append(paragraphs, "<h2>"+html.EscapeString(strings.TrimSpace(strings.TrimPrefix(line, "### ")))+"</h2>")
			continue
		}

		if strings.HasPrefix(line, "- ") {
			flush()
			// Use the literal Unicode bullet — XHTML strict only knows the 5
			// named entities, so &bull; would break epub.js parsing.
			paragraphs = append(paragraphs, "<p>• "+inlineMarkdown(strings.TrimSpace(strings.TrimPrefix(line, "- ")))+"</p>")
			continue
		}

		block = append(block, line)
	}

	flush()
	return strings.Join(paragraphs, "\n      ")
}

func renderSingleDocument(title, markdown string) string {
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
</html>`, html.EscapeString(title), html.EscapeString(title), renderParagraphs(strings.Split(markdown, "\n")))
}

func inlineMarkdown(s string) string {
	escaped := html.EscapeString(strings.TrimSpace(s))
	escaped = replacePair(escaped, "**", "<strong>", "</strong>")
	escaped = replacePair(escaped, "*", "<em>", "</em>")
	escaped = replacePair(escaped, "`", "<code>", "</code>")
	return escaped
}

func replacePair(src, marker, open, close string) string {
	for {
		start := strings.Index(src, marker)
		if start == -1 {
			return src
		}
		end := strings.Index(src[start+len(marker):], marker)
		if end == -1 {
			return src
		}
		end += start + len(marker)
		content := src[start+len(marker) : end]
		src = src[:start] + open + content + close + src[end+len(marker):]
	}
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

h1, h2 {
  font-family: "Palatino Linotype", serif;
}

p {
  margin: 0 0 1em;
}

code {
  font-family: monospace;
}`
