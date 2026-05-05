package reader

import (
	"bytes"
	"html"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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

// Safety: html.WithUnsafe is intentionally NOT enabled, so any raw HTML in
// markdown source is rendered as escaped text — the bytes goldmark emits never
// contain attacker HTML/JS, so wrapping in template.HTML at the call site is safe.
var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		extension.DefinitionList,
		extension.Typographer,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

func ParseMarkdown(title string, source []byte) Document {
	if strings.TrimSpace(title) == "" {
		title = "Untitled"
	}

	reader := text.NewReader(source)
	docNode := md.Parser().Parse(reader)

	headings := extractHeadings(docNode, source)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, docNode); err != nil {
		safe := "<pre>" + html.EscapeString(string(source)) + "</pre>"
		return Document{
			Title:    title,
			Headings: []Heading{{ID: "start", Title: title, Level: 1}},
			HTML:     template.HTML(safe),
		}
	}

	if len(headings) == 0 {
		headings = append(headings, Heading{ID: "start", Title: title, Level: 1})
	}

	return Document{
		Title:    title,
		Headings: headings,
		HTML:     template.HTML(buf.String()),
	}
}

func extractHeadings(root gmast.Node, source []byte) []Heading {
	var headings []Heading
	_ = gmast.Walk(root, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
		if !entering {
			return gmast.WalkContinue, nil
		}
		h, ok := n.(*gmast.Heading)
		if !ok {
			return gmast.WalkContinue, nil
		}

		id := ""
		if v, found := h.AttributeString("id"); found {
			switch s := v.(type) {
			case string:
				id = s
			case []byte:
				id = string(s)
			}
		}
		if id == "" {
			id = "section"
		}

		headings = append(headings, Heading{
			ID:    id,
			Title: nodeText(h, source),
			Level: h.Level,
		})
		return gmast.WalkSkipChildren, nil
	})
	return headings
}

func nodeText(node gmast.Node, source []byte) string {
	var b strings.Builder
	_ = gmast.Walk(node, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
		if !entering {
			return gmast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *gmast.Text:
			b.Write(t.Segment.Value(source))
		case *gmast.String:
			b.Write(t.Value)
		case *gmast.AutoLink:
			b.Write(t.URL(source))
		}
		return gmast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}
