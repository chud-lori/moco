package server

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"moco/internal/reader"
	"moco/internal/store"
)

type pageData struct {
	Title       string
	CurrentUser *store.User
	Error       string
	Message     string
	Nav         string // "library" | "quotes" | "stats" | "discover" | "settings"
	SEO         SEOData
}

// SEOData drives the <meta> tags rendered by the social_meta partial. All
// fields are optional — empty values fall back to sensible defaults.
type SEOData struct {
	Title       string
	Description string
	URL         string
	Image       string        // absolute or root-relative URL
	OGType      string        // "website" (default) | "article" | "book"
	JSONLD      template.HTML // raw JSON-LD payload for schema.org
}

type dashboardPageData struct {
	pageData
	MyPrivateBooks []store.Book
	MyPublicBooks  []store.Book
	PublicBooks    []store.Book
	WishlistBooks  []store.Book
	SharedBooks    []store.Book
	BookTags       map[string][]string
	AllTags        []store.TagCount
	Sort           string
	Tag            string
	Format         string
	WishlistedIDs  map[string]bool    // book IDs the current user has wishlisted
	Progress       map[string]float64 // progress_percent per book ID (missing = unread)
}

type discoverPageData struct {
	pageData
	PublicBooks   []store.Book
	WishlistedIDs map[string]bool
	Progress      map[string]float64 // progress_percent per book ID for signed-in viewers
	Query         string
	Sort          string
	Format        string
}

type authPageData struct {
	pageData
	Mode          string
	GoogleEnabled bool
	OAuthError    string // populated when callback bounces back with ?oauth_error=
}

type quotesPageData struct {
	pageData
	Quotes      []store.HighlightWithBook
	Query       string
	BookFilter  string
	BookOptions []store.Book
}

type statsPageData struct {
	pageData
	Stats     store.ReadingStats
	TagCounts []store.TagCount
}

type bookDetailPageData struct {
	pageData
	Book             store.Book
	Tags             []string
	IsOwner          bool
	IsWishlisted     bool
	ProgressPercent  float64 // 0 = unread, used for Read/Continue/Read-again CTA + cover bar
	ShareURL         string
	HasConvertedEPUB bool
}

type readerPageData struct {
	pageData
	Book           store.Book
	Document       reader.Document
	Highlights     []store.Highlight
	HighlightsJSON template.JS
	Progress       *store.ReadingProgress
	ReaderKind     string
	FileURL        string
	IsOwner        bool
	PublicAllowed  bool
}

// AssetVersion is appended to /static/* URLs as a cache-buster. Bump it on
// any deploy that changes CSS/JS so HTTP caches (Cloudflare, in-app
// browsers, mobile WebViews that ignore Cache-Control) treat the assets as
// new resources. Kept in sync with the service-worker cache key.
const AssetVersion = "v90"

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"assetVersion": func() string { return AssetVersion },
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return "Not opened yet"
			}
			return t.Format("02 Jan 2006 15:04")
		},
		"displayName": func(email string) string {
			if at := strings.Index(email, "@"); at > 0 {
				return email[:at]
			}
			if email == "" {
				return "a reader"
			}
			return email
		},
		"formatMinutes": func(m int) string {
			if m <= 0 {
				return ""
			}
			if m < 60 {
				return fmt.Sprintf("%d min read", m)
			}
			h := m / 60
			rem := m % 60
			if rem == 0 {
				return fmt.Sprintf("%dh read", h)
			}
			return fmt.Sprintf("%dh %dm read", h, rem)
		},
		"userDisplayName": func(u *store.User) string {
			if u == nil {
				return "a reader"
			}
			if u.DisplayName != "" {
				return u.DisplayName
			}
			if at := strings.Index(u.Email, "@"); at > 0 {
				return u.Email[:at]
			}
			return u.Email
		},
		// ownerName resolves a book owner's name preferring the live display
		// name, falling back to the email local-part, then "a reader".
		"ownerName": func(displayName, email string) string {
			if strings.TrimSpace(displayName) != "" {
				return displayName
			}
			if at := strings.Index(email, "@"); at > 0 {
				return email[:at]
			}
			if email == "" {
				return "a reader"
			}
			return email
		},
		"tagsForBook": func(tags map[string][]string, id string) []string {
			if tags == nil {
				return nil
			}
			return tags[id]
		},
		"isWishlisted": func(ids map[string]bool, id string) bool {
			if ids == nil {
				return false
			}
			return ids[id]
		},
		// bookProgress returns progress_percent for a book ID, or 0 if the
		// user has no row in reading_progress (= unread). Safe on nil maps
		// so guests / handlers that skip the lookup still render cleanly.
		"bookProgress": func(progress map[string]float64, id string) float64 {
			if progress == nil {
				return 0
			}
			return progress[id]
		},
		// readLabel maps a progress percentage to the verb on the primary
		// CTA. The thresholds match the cover-bar's visibility threshold so
		// "Continue reading" never shows on a card with no visible progress
		// indicator.
		"readLabel": func(pct float64) string {
			switch {
			case pct >= 95:
				return "Read again"
			case pct > 0.5:
				return "Continue reading"
			}
			return "Read"
		},
		// mkBookGrid bundles a book slice + the user's book→tags map +
		// per-book progress so the "book_grid" sub-template gets everything
		// as a single argument.
		"mkBookGrid": func(books []store.Book, tags map[string][]string, progress map[string]float64) bookGridData {
			return bookGridData{Books: books, Tags: tags, Progress: progress}
		},
	}
}

type bookGridData struct {
	Books    []store.Book
	Tags     map[string][]string
	Progress map[string]float64
}
