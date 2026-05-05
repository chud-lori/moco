package server

import (
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
}

type dashboardPageData struct {
	pageData
	MyPrivateBooks []store.Book
	MyPublicBooks  []store.Book
	PublicBooks    []store.Book
}

type discoverPageData struct {
	pageData
	PublicBooks []store.Book
}

type authPageData struct {
	pageData
	Mode string
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
	}
}
