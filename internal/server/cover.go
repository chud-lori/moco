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

	seed := hashSeed(book.ID + title)
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
	lines, fontSize := wrapTitle(title, coverBandedTitleWidthChars, 4)
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
	lines, fontSize := wrapTitle(title, coverStripeTitleWidthChars, 4)
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
	lines, fontSize := wrapTitle(title, coverFrameTitleWidthChars, 4)
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

const (
	coverBandedTitleWidthChars = 16
	coverStripeTitleWidthChars = 18
	coverFrameTitleWidthChars  = 20
)

// wrapTitle splits a title into up to maxLines, returning the lines plus a
// font size sized down so longer titles still fit on the canvas.
func wrapTitle(title string, maxChars, maxLines int) ([]string, int) {
	words := strings.Fields(title)
	var lines []string
	var current []string
	currentLen := 0
	for _, w := range words {
		if len(w) > maxChars && len(current) == 0 {
			lines = append(lines, w)
			continue
		}
		if currentLen+len(w)+1 > maxChars && len(current) > 0 {
			lines = append(lines, strings.Join(current, " "))
			current = []string{w}
			currentLen = len(w)
			continue
		}
		current = append(current, w)
		if currentLen == 0 {
			currentLen = len(w)
		} else {
			currentLen += len(w) + 1
		}
	}
	if len(current) > 0 {
		lines = append(lines, strings.Join(current, " "))
	}
	if len(lines) == 0 {
		lines = []string{title}
	}
	if len(lines) > maxLines {
		// Keep first maxLines-1 lines, fold the rest into the last with ellipsis.
		merged := strings.Join(lines[maxLines-1:], " ")
		if len(merged) > maxChars-1 {
			merged = strings.TrimRight(merged[:maxChars-1], " ") + "…"
		}
		lines = append(lines[:maxLines-1], merged)
	}
	font := 180
	switch len(lines) {
	case 1:
		font = 200
	case 2:
		font = 170
	case 3:
		font = 140
	default:
		font = 120
	}
	return lines, font
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
