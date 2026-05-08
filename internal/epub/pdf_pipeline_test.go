package epub

import "testing"

func TestClassifyPage(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		imageCount int
		want       pageProfile
	}{
		{
			name:       "scanned image page with almost no text",
			text:       "",
			imageCount: 1,
			want:       pageScanned,
		},
		{
			name:       "digital text page",
			text:       "This chapter has plenty of readable text and very little noise on the page for extraction.",
			imageCount: 0,
			want:       pageDigital,
		},
		{
			name:       "mixed page with text and images",
			text:       "This page contains enough readable text to be considered mixed alongside extracted diagrams and figures embedded in the PDF layout, including captions, side notes, longer explanatory paragraphs, section labels, table introductions, repeated references, and narrative context that make the content clearly more than a scanned image alone and substantially more than a short OCR fragment.",
			imageCount: 2,
			want:       pageMixed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPage(tt.text, tt.imageCount); got != tt.want {
				t.Fatalf("classifyPage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScoreStructuredMarkdown(t *testing.T) {
	good := "# Chapter One\n\nA full paragraph with useful structure.\n\n- item one\n- item two\n\n| A | B |\n| - | - |\n| 1 | 2 |\n"
	bad := "� � �\n\nx\nx\nx\n"

	if scoreStructuredMarkdown(good) <= scoreStructuredMarkdown(bad) {
		t.Fatalf("expected structured markdown score to prefer well-formed output")
	}
}
