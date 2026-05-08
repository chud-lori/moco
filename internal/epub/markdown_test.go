package epub

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestMarkdownToEPUBPreservesStructure(t *testing.T) {
	src := strings.Join([]string{
		"---",
		"title: Front Matter Title",
		"author: Example Author",
		"---",
		"",
		"# Chapter One",
		"",
		"Intro with a [safe link](https://example.com).",
		"",
		"- first item",
		"- second item",
		"",
		"| Name | Value |",
		"| --- | ---: |",
		"| one | 1 |",
		"",
		"```go",
		`fmt.Println("hello")`,
		"```",
		"",
	}, "\n")

	data, err := MarkdownToEPUB("", "", []byte(src))
	if err != nil {
		t.Fatalf("MarkdownToEPUB returned error: %v", err)
	}

	files := unzipEPUB(t, data)

	opf := files["OEBPS/content.opf"]
	if !strings.Contains(opf, "<dc:title>Front Matter Title</dc:title>") {
		t.Fatalf("content.opf missing frontmatter title: %s", opf)
	}
	if !strings.Contains(opf, "<dc:creator>Example Author</dc:creator>") {
		t.Fatalf("content.opf missing frontmatter author: %s", opf)
	}

	chapter := files["OEBPS/chapter-01.xhtml"]
	for _, want := range []string{
		"<h1>Chapter One</h1>",
		"<a href=\"https://example.com\">safe link</a>",
		"<ul>",
		"<table>",
		"<code class=\"language-go\">",
	} {
		if !strings.Contains(chapter, want) {
			t.Fatalf("chapter output missing %q:\n%s", want, chapter)
		}
	}
}

func unzipEPUB(t *testing.T, body []byte) map[string]string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	files := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", file.Name, err)
		}
		files[file.Name] = string(content)
	}
	return files
}
