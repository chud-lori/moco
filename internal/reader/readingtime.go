package reader

import (
	"strings"
	"unicode"
)

// EstimateMinutesFromMarkdown counts words and divides by an average reading
// pace. Used at upload time so the dashboard can show "~12 min read" without
// re-parsing the file on every render.
func EstimateMinutesFromMarkdown(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	words := 0
	inWord := false
	for _, r := range string(source) {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return minutesFromWords(words)
}

// EstimateMinutesFromBytes is a coarse fallback for formats whose word count
// we don't compute server-side (PDF, EPUB). It assumes ~6 chars/word and
// 250 wpm reading pace.
func EstimateMinutesFromBytes(format string, fileSize int64) int {
	if fileSize <= 0 {
		return 0
	}
	// Tune the fudge factor per format — PDFs and EPUBs include zip
	// overhead, fonts, images that aren't reading content.
	switch strings.ToLower(format) {
	case "pdf":
		// ~1.6KB of "actual content" per A4 page, ~250 words/page, 250 wpm → 1 min/page
		return clamp(int(fileSize / 1600))
	case "epub":
		// EPUBs are zipped XHTML; a typical novel is ~600KB for ~6 hours of reading
		return clamp(int(fileSize / 1700))
	default:
		// markdown / unknown — 6 chars/word, 250 wpm
		return clamp(int(fileSize / 1500))
	}
}

func minutesFromWords(words int) int {
	if words <= 0 {
		return 0
	}
	const wpm = 250
	m := (words + wpm/2) / wpm
	if m < 1 {
		m = 1
	}
	return m
}

func clamp(m int) int {
	if m < 1 {
		return 1
	}
	if m > 60*100 {
		return 60 * 100 // 100h ceiling — sanity guard
	}
	return m
}
