package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"moco/internal/store"
)

// Atom 1.0 feed for the public shelf. Strangers who land on /discover get a
// "subscribe" affordance via the auto-discovery <link rel="alternate"> in the
// page head and the visible footer link. RSS readers (NetNewsWire, Reeder,
// Inoreader, etc.) all accept Atom — no need to ship RSS 2.0 alongside.

const feedEntryLimit = 30

type atomFeed struct {
	XMLName  xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle,omitempty"`
	Links    []atomLink  `xml:"link"`
	ID       string      `xml:"id"`
	Updated  string      `xml:"updated"`
	Author   *atomPerson `xml:"author,omitempty"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomPerson struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string      `xml:"title"`
	Links     []atomLink  `xml:"link"`
	ID        string      `xml:"id"`
	Published string      `xml:"published"`
	Updated   string      `xml:"updated"`
	Author    *atomPerson `xml:"author,omitempty"`
	Summary   *atomText   `xml:"summary,omitempty"`
	Category  *atomCat    `xml:"category,omitempty"`
}

type atomText struct {
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type atomCat struct {
	Term string `xml:"term,attr"`
}

// handleFeed serves the Atom feed of the latest public books. Aggregators
// poll this frequently — we keep it cheap by issuing the same query the
// /discover page uses (one DB hit) and capping at feedEntryLimit entries.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	books, err := s.store.ListPublicBooks(r.Context(), "", "", "", "")
	if err != nil {
		http.Error(w, "failed to load feed", http.StatusInternalServerError)
		return
	}
	if len(books) > feedEntryLimit {
		books = books[:feedEntryLimit]
	}

	feedURL := s.absoluteURL(r, "/feed.xml")
	siteURL := s.absoluteURL(r, "/discover")
	// updated is the newest entry's UpdatedAt — falls back to "now" so an
	// empty shelf still renders a valid feed.
	updated := time.Now().UTC()
	if len(books) > 0 {
		updated = books[0].UpdatedAt
		for _, b := range books[1:] {
			if b.UpdatedAt.After(updated) {
				updated = b.UpdatedAt
			}
		}
	}

	feed := atomFeed{
		Title:    "Moco — public shelf",
		Subtitle: "Books published on Moco's public shelf",
		Links: []atomLink{
			{Rel: "self", Type: "application/atom+xml", Href: feedURL},
			{Rel: "alternate", Type: "text/html", Href: siteURL},
		},
		ID:      siteURL,
		Updated: updated.UTC().Format(time.RFC3339),
		Entries: make([]atomEntry, 0, len(books)),
	}

	for _, b := range books {
		entryURL := s.absoluteURL(r, "/books/"+b.ID)
		entry := atomEntry{
			Title: b.Title,
			Links: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: entryURL},
			},
			ID:        entryURL,
			Published: b.CreatedAt.UTC().Format(time.RFC3339),
			Updated:   b.UpdatedAt.UTC().Format(time.RFC3339),
		}
		// Author name: the book's stated author. If the book has no
		// author, fall back to the publisher (resolved owner name) so
		// the entry still has a person attached — RSS readers tend to
		// gray out author-less entries.
		switch {
		case b.Author != "":
			entry.Author = &atomPerson{Name: b.Author}
		case !b.AnonymousOwner:
			entry.Author = &atomPerson{Name: ownerNameFromBook(b)}
		}
		// Summary: real description if we have one, otherwise a
		// generic line so feed previews aren't blank.
		if d := ogDescriptionForBook(b, ""); d != "" {
			entry.Summary = &atomText{Type: "text", Content: d}
		}
		if b.Format != "" {
			entry.Category = &atomCat{Term: b.Format}
		}
		feed.Entries = append(feed.Entries, entry)
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	// 5-minute edge cache — feeds are polled frequently and book uploads
	// are rare, so this is mostly a CDN-load reducer.
	w.Header().Set("Cache-Control", "public, max-age=300")
	if _, err := fmt.Fprint(w, xml.Header); err != nil {
		return
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(feed)
}

// ownerNameFromBook resolves a display name for the book's publisher using
// the same precedence as the ownerName template helper: display name >
// email local-part > "a reader".
func ownerNameFromBook(b store.Book) string {
	if b.OwnerDisplayName != "" {
		return b.OwnerDisplayName
	}
	if b.OwnerEmail != "" {
		for i, c := range b.OwnerEmail {
			if c == '@' {
				return b.OwnerEmail[:i]
			}
		}
		return b.OwnerEmail
	}
	return "a reader"
}
