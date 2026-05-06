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
	BookTags       map[string][]string
	AllTags        []store.TagCount
	Sort           string
	Tag            string
	Format         string
	WishlistedIDs  map[string]bool // book IDs the current user has wishlisted
}

type discoverPageData struct {
	pageData
	PublicBooks   []store.Book
	WishlistedIDs map[string]bool
}

type authPageData struct {
	pageData
	Mode string
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

func templateFuncs() template.FuncMap {
	return template.FuncMap{
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
		// mkBookGrid bundles a book slice + the user's book→tags map so the
		// "book_grid" sub-template gets both as a single argument.
		"mkBookGrid": func(books []store.Book, tags map[string][]string) bookGridData {
			return bookGridData{Books: books, Tags: tags}
		},
	}
}

type bookGridData struct {
	Books []store.Book
	Tags  map[string][]string
}
