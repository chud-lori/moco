package reader

import (
	"fmt"
	"html"
	"html/template"
	"strings"
)

type Heading struct {
	ID    string
	Title string
	Level int
}

type Document struct {
	Title    string
	Headings []Heading
	HTML     template.HTML
}

func ParseMarkdown(title string, source []byte) Document {
	lines := strings.Split(string(source), "\n")
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}

	var headings []Heading
	var blocks []string
	var paragraph []string
	sectionIdx := 0

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = append(blocks, "<p>"+inlineMarkdown(strings.Join(paragraph, " "))+"</p>")
		paragraph = nil
	}

	addHeading := func(level int, raw string) {
		flushParagraph()
		sectionIdx++
		text := strings.TrimSpace(raw)
		id := slugID(text, sectionIdx)
		headings = append(headings, Heading{ID: id, Title: text, Level: level})
		tag := "h1"
		if level == 2 {
			tag = "h2"
		}
		if level >= 3 {
			tag = "h3"
		}
		blocks = append(blocks, fmt.Sprintf(`<%s id="%s">%s</%s>`, tag, id, html.EscapeString(text), tag))
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "# "):
			addHeading(1, strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			addHeading(2, strings.TrimPrefix(line, "## "))
		case strings.HasPrefix(line, "### "):
			addHeading(3, strings.TrimPrefix(line, "### "))
		case strings.HasPrefix(line, "- "):
			flushParagraph()
			blocks = append(blocks, "<p class=\"bullet\">&bull; "+inlineMarkdown(strings.TrimPrefix(line, "- "))+"</p>")
		case line == "":
			flushParagraph()
		default:
			paragraph = append(paragraph, line)
		}
	}

	flushParagraph()

	if len(headings) == 0 {
		headings = append(headings, Heading{ID: "start", Title: title, Level: 1})
		blocks = append([]string{fmt.Sprintf(`<h1 id="start">%s</h1>`, html.EscapeString(title))}, blocks...)
	}

	return Document{Title: title, Headings: headings, HTML: template.HTML(strings.Join(blocks, "\n"))}
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
		src = src[:start] + open + src[start+len(marker):end] + close + src[end+len(marker):]
	}
}

func slugID(title string, index int) string {
	normalized := strings.ToLower(strings.TrimSpace(title))
	replacer := strings.NewReplacer(" ", "-", ".", "", ",", "", "/", "-", ":", "", "'", "", "\"", "")
	normalized = replacer.Replace(normalized)
	if normalized == "" {
		normalized = "section"
	}
	return fmt.Sprintf("%s-%d", normalized, index)
}
