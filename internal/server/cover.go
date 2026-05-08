package server

import (
	"fmt"
	"strings"

	"moco/internal/store"
)

// generateCoverSVG renders a stylized 1600×2400 SVG cover when a book has no
// uploaded cover. The variant + palette are picked by hashing the book ID so
// the same book always renders the same cover, but different books look
// different.
func generateCoverSVG(book store.Book) string {
	return generateCoverSVGWithSalt(book, "")
}

// generateCoverSVGWithSalt renders the same cover but uses salt to pick the
// variant/palette so the user can re-roll the design without touching the
// rendered text. Salt is opaque to the generator — typically a counter or
// random hex.
//
// Seed selection rule:
//   - salt == "": seed = hash(book.ID + title) so each book gets a stable
//     design even after title edits.
//   - salt != "": seed = hash(salt) only. Title/author drive the rendered
//     text but not the variant/palette — editing the title in the upload
//     form just re-letters the cover, while "Try another style" re-rolls.
func generateCoverSVGWithSalt(book store.Book, salt string) string {
	title := strings.TrimSpace(book.Title)
	if title == "" {
		title = "Untitled"
	}
	author := strings.TrimSpace(book.Author)
	if author == "" {
		author = strings.TrimSpace(book.OwnerDisplayName)
	}
	if author == "" {
		author = ownerLocalPart(book.OwnerEmail)
	}

	var seed uint32
	if salt == "" {
		seed = hashSeed(book.ID + title)
	} else {
		seed = hashSeed(salt)
	}
	palette := coverPalettes[seed%uint32(len(coverPalettes))]
	variant := (seed / 7) % uint32(len(coverVariants))

	switch coverVariants[variant] {
	case "banded":
		return renderBandedCover(palette, title, author, book.Format)
	case "stripe":
		return renderStripeCover(palette, title, author, book.Format)
	default:
		return renderFrameCover(palette, title, author, book.Format)
	}
}

type coverPalette struct {
	primary, primaryDark, paper, ink, muted string
}

var coverPalettes = []coverPalette{
	{primary: "#1f3a5f", primaryDark: "#0d1f33", paper: "#f5f1e8", ink: "#111111", muted: "#444444"}, // navy / cream
	{primary: "#2f5042", primaryDark: "#1c3328", paper: "#efe9d9", ink: "#1d2120", muted: "#3d5145"}, // forest / linen
	{primary: "#6b2e2e", primaryDark: "#3d1717", paper: "#f5ecdf", ink: "#1f1313", muted: "#4d2d2d"}, // burgundy / ivory
	{primary: "#3a3a4f", primaryDark: "#1f1f30", paper: "#ece7d9", ink: "#16161e", muted: "#3a3a4a"}, // slate / parchment
	{primary: "#9a4d2e", primaryDark: "#5a2916", paper: "#f0e6d2", ink: "#1f1310", muted: "#3a261c"}, // terracotta / sand
	{primary: "#2b4f5a", primaryDark: "#15323b", paper: "#eee7d6", ink: "#101a1c", muted: "#33474d"}, // teal / oat
}

var coverVariants = []string{"banded", "stripe", "frame"}

const (
	coverWidth  = 1600
	coverHeight = 2400
)

const titleFont = "Cormorant Garamond, Palatino Linotype, Georgia, serif"
const labelFont = "Manrope, Helvetica, sans-serif"

// renderBandedCover — top accent band + title block + bottom accent band with
// author. Closely mirrors the user-supplied SVG sample.
func renderBandedCover(p coverPalette, title, author, format string) string {
	lines, fontSize := wrapTitle(title, 4)
	titleSVG := renderTitleLines(lines, 100, 760, fontSize, 1.18, p.ink)

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">
  <rect width="%d" height="%d" fill="%s"/>
  <rect x="0" y="0" width="%d" height="380" fill="%s"/>
  <rect x="0" y="380" width="%d" height="6" fill="%s"/>
  <text x="100" y="200" font-family="%s" font-size="64" font-style="italic" fill="%s">A reading from</text>
  <text x="100" y="290" font-family="%s" font-size="64" font-style="italic" fill="%s">%s</text>
  %s
  <rect x="100" y="1480" width="220" height="3" fill="%s"/>
  <text x="100" y="1580" font-family="%s" font-size="44" fill="%s">%s edition</text>
  <rect x="0" y="2200" width="%d" height="6" fill="%s"/>
  <rect x="0" y="2206" width="%d" height="194" fill="%s"/>
  <text x="100" y="2330" font-family="%s" font-size="60" font-weight="600" fill="%s">%s</text>
  <text x="%d" y="2330" font-family="%s" font-size="38" font-style="italic" fill="%s" text-anchor="end">moco</text>
</svg>`,
		coverWidth, coverHeight, coverWidth, coverHeight,
		coverWidth, coverHeight, p.paper,
		coverWidth, p.primary,
		coverWidth, p.primaryDark,
		titleFont, p.paper,
		titleFont, p.paper, htmlEscape(displayOwner(author, "Moco library")),
		titleSVG,
		p.primary,
		titleFont, p.muted, htmlEscape(strings.ToUpper(displayFormat(format))),
		coverWidth, p.primaryDark,
		coverWidth, p.primary,
		titleFont, p.paper, htmlEscape(displayOwner(author, "Anonymous")),
		coverWidth-100, titleFont, p.paper,
	)
}

// renderStripeCover — vertical accent stripe down the left, title and author
// in the open space.
func renderStripeCover(p coverPalette, title, author, format string) string {
	lines, fontSize := wrapTitle(title, 4)
	titleSVG := renderTitleLines(lines, 220, 900, fontSize, 1.18, p.ink)

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">
  <rect width="%d" height="%d" fill="%s"/>
  <rect x="0" y="0" width="120" height="%d" fill="%s"/>
  <rect x="120" y="0" width="6" height="%d" fill="%s"/>
  <text x="220" y="240" font-family="%s" font-size="48" letter-spacing="6" fill="%s">%s · MOCO</text>
  %s
  <rect x="220" y="1500" width="240" height="3" fill="%s"/>
  <text x="220" y="2280" font-family="%s" font-size="56" font-weight="600" fill="%s">%s</text>
</svg>`,
		coverWidth, coverHeight, coverWidth, coverHeight,
		coverWidth, coverHeight, p.paper,
		coverHeight, p.primary,
		coverHeight, p.primaryDark,
		labelFont, p.muted, htmlEscape(strings.ToUpper(displayFormat(format))),
		titleSVG,
		p.primary,
		titleFont, p.ink, htmlEscape(displayOwner(author, "Anonymous")),
	)
}

// renderFrameCover — inset frame border, title centered, author at bottom.
func renderFrameCover(p coverPalette, title, author, format string) string {
	lines, fontSize := wrapTitle(title, 4)
	startY := coverHeight/2 - (len(lines)*int(float64(fontSize)*1.15))/2 + int(float64(fontSize)*0.3)
	titleSVG := renderTitleLinesCentered(lines, coverWidth/2, startY, fontSize, 1.15, p.ink)

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">
  <rect width="%d" height="%d" fill="%s"/>
  <rect x="80" y="80" width="%d" height="%d" fill="none" stroke="%s" stroke-width="6"/>
  <rect x="120" y="120" width="%d" height="%d" fill="none" stroke="%s" stroke-width="2"/>
  <text x="%d" y="280" font-family="%s" font-size="48" letter-spacing="8" fill="%s" text-anchor="middle">%s</text>
  %s
  <rect x="%d" y="%d" width="240" height="3" fill="%s"/>
  <text x="%d" y="2240" font-family="%s" font-size="56" font-weight="600" fill="%s" text-anchor="middle">%s</text>
  <text x="%d" y="2310" font-family="%s" font-size="36" font-style="italic" fill="%s" text-anchor="middle">moco library</text>
</svg>`,
		coverWidth, coverHeight, coverWidth, coverHeight,
		coverWidth, coverHeight, p.paper,
		coverWidth-160, coverHeight-160, p.primary,
		coverWidth-240, coverHeight-240, p.primaryDark,
		coverWidth/2, labelFont, p.primary, htmlEscape(strings.ToUpper(displayFormat(format))),
		titleSVG,
		coverWidth/2-120, coverHeight-340, p.primary,
		coverWidth/2, titleFont, p.ink, htmlEscape(displayOwner(author, "Anonymous")),
		coverWidth/2, titleFont, p.muted,
	)
}

// ----- helpers -----

// wrapTitle picks the smallest line count that fits the title on the canvas,
// returning those lines plus the matching font size. Char budget per line
// is derived from the candidate font size — bigger font means fewer chars
// per line. Earlier versions used a single max-chars value for all line
// counts, which let titles render fine in the 1-line/2-line decision but
// overflow once a smaller font scaled char widths down.
//
// The maxLines argument is the hard cap; any title that still doesn't fit
// at the smallest font is wrapped at that font and the last line is
// truncated with an ellipsis.
func wrapTitle(title string, maxLines int) ([]string, int) {
	title = strings.TrimSpace(title)
	if title == "" {
		return []string{"Untitled"}, 200
	}
	words := strings.Fields(title)
	if len(words) == 0 {
		return []string{title}, 200
	}

	// Calibrated against Cormorant Garamond / Palatino bold rendered against
	// the 1400px inner width (1600 viewBox - 100px each side margin). Char
	// width ≈ 0.48 × font-size for serif bold.
	candidates := []struct{ font, maxChars int }{
		{200, 14},
		{170, 17},
		{140, 21},
		{110, 27},
	}
	if maxLines < 1 {
		maxLines = 4
	}

	for i, c := range candidates {
		targetLines := i + 1
		if targetLines > maxLines {
			break
		}
		lines := greedyWrap(words, c.maxChars)
		if len(lines) <= targetLines && longestLineLen(lines) <= c.maxChars {
			return lines, c.font
		}
	}

	// Worst case: still doesn't fit at the smallest font. Wrap aggressively
	// and ellipsis the last line so it never overflows.
	last := candidates[len(candidates)-1]
	lines := greedyWrap(words, last.maxChars)
	if len(lines) > maxLines {
		merged := strings.Join(lines[maxLines-1:], " ")
		if len(merged) > last.maxChars-1 {
			merged = strings.TrimRight(merged[:last.maxChars-1], " ") + "…"
		}
		lines = append(lines[:maxLines-1], merged)
	}
	return lines, last.font
}

// greedyWrap breaks `words` into the fewest lines such that no line exceeds
// `maxChars`. A word longer than maxChars goes on its own line.
func greedyWrap(words []string, maxChars int) []string {
	var lines []string
	var current []string
	currentLen := 0
	for _, w := range words {
		// Lone over-long word: emit on its own line.
		if len(w) > maxChars && len(current) == 0 {
			lines = append(lines, w)
			continue
		}
		next := currentLen + len(w)
		if currentLen > 0 {
			next++ // space
		}
		if next > maxChars && len(current) > 0 {
			lines = append(lines, strings.Join(current, " "))
			current = []string{w}
			currentLen = len(w)
			continue
		}
		current = append(current, w)
		currentLen = next
	}
	if len(current) > 0 {
		lines = append(lines, strings.Join(current, " "))
	}
	return lines
}

func longestLineLen(lines []string) int {
	n := 0
	for _, l := range lines {
		if len(l) > n {
			n = len(l)
		}
	}
	return n
}

func renderTitleLines(lines []string, x, y, size int, lineHeight float64, ink string) string {
	var sb strings.Builder
	for i, line := range lines {
		ly := y + int(float64(i)*float64(size)*lineHeight)
		fmt.Fprintf(&sb, `<text x="%d" y="%d" font-family="%s" font-size="%d" font-weight="700" fill="%s">%s</text>`,
			x, ly, titleFont, size, ink, htmlEscape(line))
	}
	return sb.String()
}

func renderTitleLinesCentered(lines []string, cx, y, size int, lineHeight float64, ink string) string {
	var sb strings.Builder
	for i, line := range lines {
		ly := y + int(float64(i)*float64(size)*lineHeight)
		fmt.Fprintf(&sb, `<text x="%d" y="%d" font-family="%s" font-size="%d" font-weight="700" fill="%s" text-anchor="middle">%s</text>`,
			cx, ly, titleFont, size, ink, htmlEscape(line))
	}
	return sb.String()
}

// hashSeed: stable 32-bit hash of a string. Same formula as the prior
// placeholder, just widened so we have more bits to mix into variant + palette.
func hashSeed(s string) uint32 {
	var h uint32 = 2166136261
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}

func displayFormat(f string) string {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "":
		return "book"
	default:
		return f
	}
}

func displayOwner(author, fallback string) string {
	a := strings.TrimSpace(author)
	if a == "" {
		return fallback
	}
	if len(a) > 36 {
		return a[:33] + "…"
	}
	return a
}

func ownerLocalPart(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return ""
}
