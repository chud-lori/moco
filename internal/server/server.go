package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"moco/internal/auth"
	"moco/internal/epub"
	"moco/internal/reader"
	"moco/internal/storage"
	"moco/internal/store"
)

//go:embed web/templates/*.html web/static/*
var embeddedFiles embed.FS

// maxUploadBytes caps multipart request bodies for upload + inspect endpoints.
// 120MB ≈ client-side limit (100MB book) + cover (~5MB) + multipart overhead.
const maxUploadBytes = 120 << 20

type Config struct {
	Addr           string
	DataDir        string
	DBPath         string
	CookieName     string
	SecureCookies  bool
	PublicURL      string // canonical https://... URL — used for absolute links in OG tags
	Storage        storage.Backend
	StorageBaseDir string // local fallback dir when backend is filesystem-based
}

type Server struct {
	cfg       Config
	mux       *http.ServeMux
	templates *template.Template
	store     *store.Store
	storage   storage.Backend
}

func New(cfg Config) *Server {
	if cfg.DataDir == "" {
		cfg.DataDir = "var"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "moco.sqlite")
	}
	if cfg.CookieName == "" {
		cfg.CookieName = "moco_session"
	}

	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "books"), 0o755); err != nil {
		panic(err)
	}

	dbStore, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		panic(err)
	}

	if cfg.Storage == nil {
		cfg.Storage = storage.NewLocal(cfg.DataDir)
	}

	s := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		store:   dbStore,
		storage: cfg.Storage,
	}

	s.templates = template.Must(template.New("").Funcs(templateFuncs()).ParseFS(embeddedFiles, "web/templates/*.html"))
	s.routes()
	go runTempCleanupWorker(context.Background())
	return s
}

// serveBackendObject streams an object from the storage backend to the
// response. For the local backend it short-circuits to http.ServeFile (which
// supports range requests, useful for pdf.js). For remote backends it does a
// simple streaming Get.
func (s *Server) serveBackendObject(w http.ResponseWriter, r *http.Request, key, contentType, filename string) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	}
	if path := storage.LocalPathOf(s.storage, key); path != "" {
		http.ServeFile(w, r, path)
		return
	}
	rc, err := s.storage.Get(r.Context(), key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer rc.Close()
	if size, ok, _ := s.storage.Stat(r.Context(), key); ok && size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	_, _ = io.Copy(w, rc)
}

// keyForBook returns the canonical storage key for a piece of a book.
func keyForBook(userID, bookID, name string) string {
	return fmt.Sprintf("books/%s/%s/%s", userID, bookID, name)
}

// runTempCleanupWorker scans the OS temp dir hourly and removes Moco's
// conversion / inspect artefacts older than 24h. Keeps the host disk lean.
func runTempCleanupWorker(ctx context.Context) {
	const ttl = 24 * time.Hour
	const interval = time.Hour
	prefixes := []string{"moco-pdf-", "moco-pdf-cal-", "moco-pdf-mu-", "moco-cover-", "moco-inspect-"}

	sweep := func() {
		entries, err := os.ReadDir(os.TempDir())
		if err != nil {
			return
		}
		now := time.Now()
		removed := 0
		for _, e := range entries {
			matched := false
			for _, p := range prefixes {
				if strings.HasPrefix(e.Name(), p) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) < ttl {
				continue
			}
			full := filepath.Join(os.TempDir(), e.Name())
			if err := os.RemoveAll(full); err == nil {
				removed++
			}
		}
		if removed > 0 {
			log.Printf("temp cleanup: removed %d stale artefacts", removed)
		}
	}

	sweep() // run once at boot
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// MigrateLocalToBackend copies every book file referenced in the DB from the
// local filesystem (var/books/...) into the configured Storage backend, then
// rewrites the DB rows to use the new key format. Idempotent — files already
// at their target keys are skipped.
func (s *Server) MigrateLocalToBackend(ctx context.Context) error {
	if s.storage == nil {
		return errors.New("no storage backend configured")
	}
	books, err := s.store.AllBooksAdmin(ctx)
	if err != nil {
		return err
	}
	for _, b := range books {
		if err := s.migrateOneBook(ctx, b); err != nil {
			log.Printf("migrate book %s: %v", b.ID, err)
		}
	}
	return nil
}

func (s *Server) migrateOneBook(ctx context.Context, b store.Book) error {
	moves := []*string{&b.StoragePath, &b.DerivedEPUBPath, &b.CoverPath}
	for _, p := range moves {
		path := *p
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) && !strings.HasPrefix(path, s.cfg.DataDir) {
			continue // already a logical key
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		key := localPathToKey(s.cfg.DataDir, path)
		ct := mime.TypeByExtension(filepath.Ext(path))
		err = s.storage.Put(ctx, key, f, ct, 0)
		f.Close()
		if err != nil {
			return err
		}
		*p = key
	}
	return s.store.UpdateBookPaths(ctx, b.ID, b.StoragePath, b.DerivedEPUBPath, b.CoverPath)
}

func localPathToKey(dataDir, abs string) string {
	rel, err := filepath.Rel(dataDir, abs)
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

func (s *Server) Handler() http.Handler {
	handler := http.Handler(s.mux)
	handler = s.withCSRFCookie(handler)
	handler = s.withSecurityHeaders(handler)
	return s.withLogging(handler)
}

func (s *Server) routes() {
	staticFS, err := fs.Sub(embeddedFiles, "web/static")
	if err != nil {
		panic(err)
	}

	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	s.mux.HandleFunc("GET /", s.handleHome)
	s.mux.HandleFunc("GET /discover", s.handleDiscover)
	s.mux.HandleFunc("GET /signup", s.handleSignupPage)
	s.mux.HandleFunc("GET /login", s.handleLoginPage)
	s.mux.HandleFunc("GET /app", s.handleDashboard)
	s.mux.HandleFunc("GET /quotes", s.handleQuotes)
	s.mux.HandleFunc("GET /stats", s.handleStats)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("GET /books/{id}", s.handleBookDetail)
	s.mux.HandleFunc("GET /books/{id}/read", s.handleReadBook)

	// PWA assets
	s.mux.HandleFunc("GET /manifest.webmanifest", s.handleManifest)
	s.mux.HandleFunc("GET /sw.js", s.handleServiceWorker)

	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/v1/auth/signup", s.handleSignup)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("PUT /api/v1/auth/me", s.handleUpdateAccount)
	s.mux.HandleFunc("PUT /api/v1/auth/password", s.handleChangePassword)
	s.mux.HandleFunc("DELETE /api/v1/auth/me", s.handleDeleteAccount)
	s.mux.HandleFunc("GET /api/v1/books", s.handleBooks)
	s.mux.HandleFunc("GET /api/v1/books/public", s.handlePublicBooks)
	s.mux.HandleFunc("POST /api/v1/books/upload", s.handleUploadBook)
	s.mux.HandleFunc("POST /api/v1/books/inspect", s.handleInspectBook)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/visibility", s.handleUpdateVisibility)
	s.mux.HandleFunc("PATCH /api/v1/books/{id}", s.handleUpdateBookMetadata)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/cover", s.handleUploadBookCover)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/total-pages", s.handleUpdateTotalPages)
	s.mux.HandleFunc("POST /api/v1/books/{id}/cover/regenerate", s.handleRegenerateBookCover)
	s.mux.HandleFunc("GET /api/v1/cover/preview", s.handleCoverPreview)
	s.mux.HandleFunc("GET /api/v1/books/{id}/content", s.handleServeBookContent)
	s.mux.HandleFunc("GET /api/v1/books/{id}/cover", s.handleServeBookCover)
	s.mux.HandleFunc("GET /api/v1/books/{id}/progress", s.handleGetProgress)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/progress", s.handlePutProgress)
	s.mux.HandleFunc("GET /api/v1/books/{id}/highlights", s.handleGetHighlights)
	s.mux.HandleFunc("POST /api/v1/books/{id}/highlights", s.handleCreateHighlight)
	s.mux.HandleFunc("GET /api/v1/books/{id}/bookmarks", s.handleListBookmarks)
	s.mux.HandleFunc("POST /api/v1/books/{id}/bookmarks", s.handleCreateBookmark)
	s.mux.HandleFunc("POST /api/v1/books/{id}/tags", s.handleAddTag)
	s.mux.HandleFunc("DELETE /api/v1/books/{id}/tags/{tag}", s.handleRemoveTag)
	s.mux.HandleFunc("POST /api/v1/wishlist/{id}", s.handleAddWishlist)
	s.mux.HandleFunc("DELETE /api/v1/wishlist/{id}", s.handleRemoveWishlist)
	s.mux.HandleFunc("GET /api/v1/wishlist", s.handleListWishlist)
	s.mux.HandleFunc("GET /api/v1/books/{id}/shares", s.handleListBookShares)
	s.mux.HandleFunc("POST /api/v1/books/{id}/shares", s.handleAddBookShare)
	s.mux.HandleFunc("DELETE /api/v1/books/{id}/shares/{userID}", s.handleRemoveBookShare)
	s.mux.HandleFunc("GET /api/v1/books/{id}/download", s.handleDownloadBook)
	s.mux.HandleFunc("GET /api/v1/books/{id}/converted.epub", s.handleDownloadConvertedEPUB)
	s.mux.HandleFunc("DELETE /api/v1/books/{id}", s.handleDeleteBook)
	s.mux.HandleFunc("DELETE /api/v1/highlights/{id}", s.handleDeleteHighlight)
	s.mux.HandleFunc("PUT /api/v1/highlights/{id}", s.handleUpdateHighlight)
	s.mux.HandleFunc("DELETE /api/v1/bookmarks/{id}", s.handleDeleteBookmark)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	publicBooks, _ := s.store.ListPublicBooks(r.Context(), "", "", "")
	user, _ := s.currentUser(r)
	s.renderTemplate(w, "home.html", discoverPageData{
		pageData: pageData{
			Title:       "Moco — read what you love, publish what you write",
			CurrentUser: user,
			SEO: SEOData{
				Title:       "Moco — read what you love, publish what you write",
				Description: "A reader for your library and a self-publishing platform for your writing. Upload PDF, EPUB, or Markdown — keep it private, share by email, or publish to the public shelf. Reflowable reading, highlights, and progress sync across devices.",
				URL:         s.absoluteURL(r, "/"),
				OGType:      "website",
			},
		},
		PublicBooks: takeBooks(publicBooks, 6),
	})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	// Show every public book, including the viewer's own — owners want
	// to see their published shelf the same way readers do.
	publicBooks, err := s.store.ListPublicBooks(r.Context(), "", query, sort)
	if err != nil {
		http.Error(w, "failed to load public books", http.StatusInternalServerError)
		return
	}
	var wishlisted map[string]bool
	if user != nil {
		ids := make([]string, 0, len(publicBooks))
		for _, b := range publicBooks {
			ids = append(ids, b.ID)
		}
		wishlisted, _ = s.store.WishlistedBookIDs(r.Context(), user.ID, ids)
	}

	data := discoverPageData{
		pageData: pageData{
			Title:       "Public Library - Moco",
			CurrentUser: user,
			Nav:         "discover",
			SEO: SEOData{
				Title:       "Public bookshelf — books shared by Moco readers",
				Description: "Browse books that readers chose to publish on Moco. Read straight in your browser — no signup required.",
				URL:         s.absoluteURL(r, "/discover"),
				OGType:      "website",
			},
		},
		PublicBooks:   publicBooks,
		WishlistedIDs: wishlisted,
		Query:         query,
		Sort:          sort,
	}
	if isFragmentRequest(r) {
		s.renderTemplate(w, "discover_results", data)
		return
	}
	s.renderTemplate(w, "discover.html", data)
}

func (s *Server) handleSignupPage(w http.ResponseWriter, _ *http.Request) {
	s.renderTemplate(w, "auth.html", authPageData{
		pageData: pageData{Title: "Sign up - Moco"},
		Mode:     "Sign up",
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, _ *http.Request) {
	s.renderTemplate(w, "auth.html", authPageData{
		pageData: pageData{Title: "Sign in - Moco"},
		Mode:     "Sign in",
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	q := r.URL.Query()
	filter := libraryFilterFromQuery(user.ID, q)
	books, err := s.store.ListBooksFiltered(r.Context(), filter)
	if err != nil {
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}
	bookTags, _ := s.store.AttachTagsToBooks(r.Context(), user.ID, books)
	allTags, _ := s.store.ListTagCounts(r.Context(), user.ID)
	publicBooks, _ := s.store.ListPublicBooks(r.Context(), user.ID, "", "")
	wishlistBooks, _ := s.store.ListWishlist(r.Context(), user.ID)
	sharedBooks, _ := s.store.ListBooksSharedWithUser(r.Context(), user.ID)
	privateBooks, ownPublic := splitBooksByVisibility(books)

	// Mark which books in the public-from-others list are already wishlisted.
	publicIDs := make([]string, 0, len(publicBooks))
	for _, b := range publicBooks {
		publicIDs = append(publicIDs, b.ID)
	}
	wishlistedIDs, _ := s.store.WishlistedBookIDs(r.Context(), user.ID, publicIDs)

	data := dashboardPageData{
		pageData: pageData{
			Title:       "Library - Moco",
			CurrentUser: &user,
			Nav:         "library",
		},
		MyPrivateBooks: privateBooks,
		MyPublicBooks:  ownPublic,
		PublicBooks:    publicBooks,
		WishlistBooks:  wishlistBooks,
		SharedBooks:    sharedBooks,
		BookTags:       bookTags,
		AllTags:        allTags,
		Sort:           filter.Sort,
		Tag:            filter.Tag,
		Format:         filter.Format,
		WishlistedIDs:  wishlistedIDs,
	}
	if isFragmentRequest(r) {
		s.renderTemplate(w, "library_results", data)
		return
	}
	s.renderTemplate(w, "dashboard.html", data)
}

func (s *Server) handleBookDetail(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil {
		if errors.Is(err, errNeedsLogin) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}

	desc := book.Title + " on Moco."
	if book.Author != "" {
		desc = book.Title + " by " + book.Author + " on Moco."
	}
	shareURL := s.absoluteURL(r, "/books/"+book.ID)
	jsonLD := bookJSONLD(book, shareURL)

	data := bookDetailPageData{
		pageData: pageData{
			Title:       book.Title + " - Moco",
			CurrentUser: user,
			SEO: SEOData{
				Title:       book.Title,
				Description: desc,
				URL:         shareURL,
				Image:       s.absoluteURL(r, "/api/v1/books/"+book.ID+"/cover"),
				OGType:      "book",
				JSONLD:      template.HTML(jsonLD),
			},
		},
		Book:             book,
		IsOwner:          user != nil && user.ID == book.UserID,
		ShareURL:         shareURL,
		HasConvertedEPUB: book.DerivedEPUBPath != "",
	}
	if user != nil {
		if tags, err := s.store.ListBookTags(r.Context(), user.ID, book.ID); err == nil {
			data.Tags = tags
		}
		if wished, err := s.store.IsInWishlist(r.Context(), user.ID, book.ID); err == nil {
			data.IsWishlisted = wished
		}
	}

	if isFragmentRequest(r) {
		// AJAX path: return only the inner card so the SPA modal can inject it
		// without re-rendering the page chrome.
		s.renderTemplate(w, "book_detail_card", data)
		return
	}
	s.renderTemplate(w, "book.html", data)
}

func (s *Server) handleQuotes(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	bookFilter := q.Get("book")
	quotes, err := s.store.SearchHighlights(r.Context(), user.ID, query, bookFilter)
	if err != nil {
		http.Error(w, "failed to load quotes", http.StatusInternalServerError)
		return
	}
	bookOptions, _ := s.store.ListBooks(r.Context(), user.ID)
	data := quotesPageData{
		pageData: pageData{
			Title:       "Highlights - Moco",
			CurrentUser: &user,
			Nav:         "quotes",
		},
		Quotes:      quotes,
		Query:       query,
		BookFilter:  bookFilter,
		BookOptions: bookOptions,
	}
	if isFragmentRequest(r) {
		s.renderTemplate(w, "quotes_results", data)
		return
	}
	s.renderTemplate(w, "quotes.html", data)
}

func (s *Server) handleReadBook(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil {
		if errors.Is(err, errNeedsLogin) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		http.NotFound(w, r)
		return
	}

	desc := "Reading " + book.Title + " on Moco."
	if book.Author != "" {
		desc = "Reading " + book.Title + " by " + book.Author + " on Moco."
	}
	jsonLD := bookJSONLD(book, s.absoluteURL(r, "/books/"+book.ID+"/read"))
	data := readerPageData{
		pageData: pageData{
			Title:       book.Title + " - Moco",
			CurrentUser: user,
			SEO: SEOData{
				Title:       book.Title,
				Description: desc,
				URL:         s.absoluteURL(r, "/books/"+book.ID+"/read"),
				OGType:      "book",
				JSONLD:      template.HTML(jsonLD),
			},
		},
		Book:          book,
		FileURL:       "/api/v1/books/" + book.ID + "/content",
		ReaderKind:    book.Format,
		IsOwner:       user != nil && user.ID == book.UserID,
		PublicAllowed: book.Visibility == "public",
	}

	if user != nil {
		if progress, err := s.store.GetProgress(r.Context(), user.ID, book.ID); err == nil {
			data.Progress = &progress
		}
		if highlights, err := s.store.ListHighlights(r.Context(), user.ID, book.ID); err == nil {
			data.Highlights = highlights
			if encoded, err := json.Marshal(highlights); err == nil {
				data.HighlightsJSON = template.JS(encoded)
			}
		}
	}

	if book.Format == "md" {
		rc, err := s.storage.Get(r.Context(), book.StoragePath)
		if err != nil {
			http.Error(w, "failed to load markdown", http.StatusInternalServerError)
			return
		}
		source, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			http.Error(w, "failed to load markdown", http.StatusInternalServerError)
			return
		}
		data.Document = reader.ParseMarkdown(book.Title, source)
	}

	s.renderTemplate(w, "reader.html", data)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "app": "moco"})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email is required"})
		return
	}
	if err := auth.PasswordLooksValid(req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hashing failed"})
		return
	}
	now := time.Now().UTC()
	user, err := s.store.CreateUser(r.Context(), randomID("usr"), req.Email, hash, now)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "email already exists"})
		return
	}
	if err := s.issueSession(w, r, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "session creation failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, passwordHash, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil || !auth.VerifyPassword(passwordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}
	if err := s.issueSession(w, r, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "session creation failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, _ := s.sessionToken(r)
	if token != "" {
		_ = s.store.DeleteSession(r.Context(), token)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false, "message": "no active session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": user})
}

func (s *Server) handleBooks(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	books, err := s.store.ListBooks(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load books"})
		return
	}
	privateBooks, publicBooks := splitBooksByVisibility(books)
	writeJSON(w, http.StatusOK, map[string]any{"privateItems": privateBooks, "publicItems": publicBooks})
}

func (s *Server) handlePublicBooks(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	exclude := ""
	if user != nil {
		exclude = user.ID
	}
	books, err := s.store.ListPublicBooks(r.Context(), exclude, strings.TrimSpace(r.URL.Query().Get("q")), strings.TrimSpace(r.URL.Query().Get("sort")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load public books"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": books})
}

// handleInspectBook accepts a multipart file, extracts metadata (title,
// author, format) without persisting, and returns it as JSON. Used by the
// upload form to pre-fill fields.
func (s *Server) handleInspectBook(w http.ResponseWriter, r *http.Request) {
	if _, err := s.requireUser(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "file is too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart request"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file is required"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	format := formatFromExt(ext)
	if format == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported file type"})
		return
	}

	tmpFile, err := os.CreateTemp("", "moco-inspect-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to stage file"})
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to write staged file"})
		return
	}
	tmpFile.Close()

	meta, _ := epub.ExtractMetadata(tmpPath, ext)
	if meta.Title == "" {
		meta.Title = epub.TitleFromFilename(header.Filename)
	}

	// Try cover extraction here so the upload form can show the user the
	// cover that's actually going to be used (priority: extracted > generated).
	// Without this the form's preview always showed the salt-generated SVG,
	// which was misleading when extraction would have produced a real cover.
	// Returned as a data URL to avoid a second round trip + a temp object.
	coverDataURL := ""
	if extracted, coverExt, ok := tryExtractCover(tmpPath, format); ok {
		mt := mime.TypeByExtension(coverExt)
		if mt == "" {
			mt = "image/png"
		}
		coverDataURL = "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(extracted)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"title":           meta.Title,
		"author":          meta.Author,
		"description":     meta.Description,
		"format":          format,
		"extractedCover":  coverDataURL,
		"hasCover":        coverDataURL != "",
	})
}

func (s *Server) handleUploadBook(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "file is too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart request"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file is required"})
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	format := formatFromExt(ext)
	if format == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported file type"})
		return
	}
	visibility := strings.ToLower(strings.TrimSpace(r.FormValue("visibility")))
	if visibility == "" {
		visibility = "private"
	}
	if visibility != "private" && visibility != "public" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "visibility must be private or public"})
		return
	}

	bookID := randomID("book")

	// Stage the upload to a local tmp file. We need a filesystem path for
	// metadata extraction and (optionally) conversion before pushing the
	// final artefacts to the storage backend.
	tmp, err := os.CreateTemp("", "moco-upload-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to stage file"})
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	size, copyErr := io.Copy(tmp, file)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to write file"})
		return
	}

	derivedEPUBKey := ""
	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}
	// Description: user input (form field) takes precedence; fall back to
	// EPUB <dc:description> metadata. The form auto-fills the textarea with
	// the inspect endpoint's detected value, so by the time we read it back
	// here the user has either kept the auto-detected text or replaced it.
	description := strings.TrimSpace(r.FormValue("description"))
	if description == "" {
		if metaForDesc, metaErr := epub.ExtractMetadata(tmpPath, ext); metaErr == nil {
			description = metaForDesc.Description
		}
	}
	if len(description) > 4000 {
		description = description[:4000]
	}
	convertToEPUB := r.FormValue("convertToEpub") == "1" || r.FormValue("convertToEpub") == "true"
	readingMinutes := reader.EstimateMinutesFromBytes(format, size)

	originalKey := keyForBook(user.ID, bookID, "original"+ext)
	storedFormat := format
	storedKey := originalKey
	storedMIME := safeMIME(header.Header.Get("Content-Type"), ext)
	storedSize := size
	// Always upload the original first.
	if err := putFromFile(r.Context(), s.storage, originalKey, tmpPath, storedMIME); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to upload to storage: " + err.Error()})
		return
	}

	if format == "md" {
		source, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read markdown for conversion"})
			return
		}
		readingMinutes = reader.EstimateMinutesFromMarkdown(source)
		epubBytes, convErr := epub.MarkdownToEPUB(title, author, source)
		if convErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "markdown to epub conversion failed"})
			return
		}
		derivedEPUBKey = keyForBook(user.ID, bookID, "converted.epub")
		if err := s.storage.Put(r.Context(), derivedEPUBKey, bytes.NewReader(epubBytes), "application/epub+zip", int64(len(epubBytes))); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to persist converted epub"})
			return
		}
		if convertToEPUB {
			storedFormat = "epub"
			storedKey = derivedEPUBKey
			storedMIME = "application/epub+zip"
			storedSize = int64(len(epubBytes))
		}
	} else if format == "pdf" && convertToEPUB {
		epubBytes, _, convErr := epub.PDFToEPUB(title, author, tmpPath)
		if convErr != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": "could not convert PDF to EPUB: " + convErr.Error(),
			})
			return
		}
		derivedEPUBKey = keyForBook(user.ID, bookID, "converted.epub")
		if err := s.storage.Put(r.Context(), derivedEPUBKey, bytes.NewReader(epubBytes), "application/epub+zip", int64(len(epubBytes))); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to persist converted epub"})
			return
		}
		storedFormat = "epub"
		storedKey = derivedEPUBKey
		storedMIME = "application/epub+zip"
		storedSize = int64(len(epubBytes))
	}

	// Cover handling: user upload first, then auto-extract from the source.
	coverKey := ""
	if cover, coverHeader, cerr := r.FormFile("cover"); cerr == nil {
		defer cover.Close()
		coverExt := strings.ToLower(filepath.Ext(coverHeader.Filename))
		if coverExt == ".jpg" || coverExt == ".jpeg" || coverExt == ".png" || coverExt == ".webp" || coverExt == ".gif" {
			coverKey = keyForBook(user.ID, bookID, "cover"+coverExt)
			coverMIME := mime.TypeByExtension(coverExt)
			if err := s.storage.Put(r.Context(), coverKey, cover, coverMIME, 0); err != nil {
				coverKey = "" // fall through to auto-extract / placeholder
			}
		}
	}
	// Skip extraction when the user ticked "Use this generated cover instead
	// of the file's first page" — they explicitly want the SVG below.
	coverForce := r.FormValue("coverForce") == "1"
	if coverKey == "" && !coverForce {
		if extracted, coverExt, ok := tryExtractCover(tmpPath, format); ok {
			coverKey = keyForBook(user.ID, bookID, "cover"+coverExt)
			if err := s.storage.Put(r.Context(), coverKey, bytes.NewReader(extracted), mime.TypeByExtension(coverExt), int64(len(extracted))); err != nil {
				coverKey = ""
			}
		}
	}
	// If we still have no cover and the upload form picked a generated
	// variant, persist the SVG so the preview the user saw on the form is
	// what actually shows up afterwards. Without this, the cover endpoint's
	// fallback would re-derive a cover from the book ID and might not match
	// what was previewed.
	if coverKey == "" {
		if salt := strings.TrimSpace(r.FormValue("coverSalt")); salt != "" {
			previewBook := store.Book{ID: bookID, Title: title, Author: author, Format: storedFormat}
			svg := generateCoverSVGWithSalt(previewBook, salt)
			coverKey = keyForBook(user.ID, bookID, "cover.svg")
			if err := s.storage.Put(r.Context(), coverKey, strings.NewReader(svg), "image/svg+xml", int64(len(svg))); err != nil {
				coverKey = ""
			}
		}
	}

	now := time.Now().UTC()
	book := store.Book{
		ID:               bookID,
		UserID:           user.ID,
		Title:            strings.ReplaceAll(title, "%20", " "),
		Author:           author,
		Description:      description,
		Format:           storedFormat,
		Visibility:       visibility,
		OwnerEmail:       user.Email,
		ReadingMinutes:   readingMinutes,
		StoragePath:      storedKey,
		OriginalFilename: header.Filename,
		MIMEType:         storedMIME,
		DerivedEPUBPath:  derivedEPUBKey,
		CoverPath:        coverKey,
		FileSize:         storedSize,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.store.CreateBook(r.Context(), book); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save book"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"book": book,
		"conversion": map[string]any{
			"epubGenerated": derivedEPUBKey != "",
		},
	})
}

// putFromFile streams a local file into the storage backend.
func putFromFile(ctx context.Context, backend storage.Backend, key, localPath, contentType string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, _ := f.Stat()
	size := int64(0)
	if info != nil {
		size = info.Size()
	}
	return backend.Put(ctx, key, f, contentType, size)
}

func (s *Server) handleUpdateVisibility(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Visibility string `json:"visibility"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Visibility = strings.ToLower(strings.TrimSpace(req.Visibility))
	if req.Visibility != "private" && req.Visibility != "public" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "visibility must be private or public"})
		return
	}
	if err := s.store.UpdateBookVisibility(r.Context(), user.ID, r.PathValue("id"), req.Visibility); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "book not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "visibility": req.Visibility})
}

// handleUpdateBookMetadata edits the user-facing title/author fields. Owner-only.
func (s *Server) handleUpdateBookMetadata(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Title       string `json:"title"`
		Author      string `json:"author"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	title := strings.TrimSpace(req.Title)
	author := strings.TrimSpace(req.Author)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title is required"})
		return
	}
	if len(title) > 300 || len(author) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title or author is too long"})
		return
	}
	if len(description) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "description is too long (max 4000 characters)"})
		return
	}
	if err := s.store.UpdateBookMetadata(r.Context(), user.ID, r.PathValue("id"), title, author, description); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "book not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "title": title, "author": author, "description": description})
}

// handleUpdateTotalPages records the PDF's true page count, reported by
// PDF.js once the user opens the book. We use it to (a) display "X pages"
// on the book detail page and (b) replace the byte-size-based reading
// time estimate with one based on actual page count (~1.2 min/page is
// the typical text-PDF reading pace). Owner-only.
func (s *Server) handleUpdateTotalPages(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		TotalPages int `json:"totalPages"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TotalPages < 1 || req.TotalPages > 100000 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "totalPages out of range"})
		return
	}
	// 1.2 min/page is calibrated against text-heavy PDFs (250 wpm, ~300
	// words per page). It's a coarse approximation — image-heavy PDFs
	// will overestimate, dense academic PDFs may slightly under.
	readingMinutes := req.TotalPages * 12 / 10
	if readingMinutes < 1 {
		readingMinutes = 1
	}
	if err := s.store.UpdateBookTotalPages(r.Context(), user.ID, r.PathValue("id"), req.TotalPages, readingMinutes); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "could not update page count"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "totalPages": req.TotalPages, "readingMinutes": readingMinutes})
}

// handleUploadBookCover replaces the stored cover image with a fresh upload.
// Multipart form with a single "cover" file field. Owner-only.
func (s *Server) handleUploadBookCover(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	bookID := r.PathValue("id")
	book, err := s.store.GetBook(r.Context(), user.ID, bookID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "book not found"})
		return
	}
	// 10MB cap for cover uploads — way more than any reasonable JPEG/PNG.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "cover image is too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart request"})
		return
	}
	file, header, err := r.FormFile("cover")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cover file is required"})
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported image type"})
		return
	}
	newKey := keyForBook(book.UserID, book.ID, "cover"+ext)
	if err := s.storage.Put(r.Context(), newKey, file, mime.TypeByExtension(ext), 0); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save cover"})
		return
	}
	// If the previous cover lived under a different extension, remove it so
	// we don't leave orphan objects in storage.
	if book.CoverPath != "" && book.CoverPath != newKey {
		_ = s.storage.Delete(r.Context(), book.CoverPath)
	}
	if err := s.store.UpdateBookCoverPath(r.Context(), user.ID, book.ID, newKey); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save cover"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coverPath": newKey})
}

// handleRegenerateBookCover re-rolls the procedurally generated SVG cover with
// a fresh salt so each click produces a different palette/variant. The
// resulting SVG is persisted to storage so it stays stable across reloads
// (and so the cover URL stays cacheable). Owner-only.
func (s *Server) handleRegenerateBookCover(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	bookID := r.PathValue("id")
	book, err := s.store.GetBook(r.Context(), user.ID, bookID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "book not found"})
		return
	}
	salt, err := randomToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to seed cover"})
		return
	}
	svg := generateCoverSVGWithSalt(book, salt)
	newKey := keyForBook(book.UserID, book.ID, "cover.svg")
	if err := s.storage.Put(r.Context(), newKey, strings.NewReader(svg), "image/svg+xml", int64(len(svg))); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save cover"})
		return
	}
	if book.CoverPath != "" && book.CoverPath != newKey {
		_ = s.storage.Delete(r.Context(), book.CoverPath)
	}
	if err := s.store.UpdateBookCoverPath(r.Context(), user.ID, book.ID, newKey); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save cover"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "coverPath": newKey})
}

// handleCoverPreview renders an in-memory generated SVG for the upload form's
// cover preview. Auth-required (still inside requireUser-protected mux paths
// at deploy via auth middleware? — explicit check below to be safe). No DB
// lookup, no storage write. Inputs are query params: title, author, format,
// salt. The salt lets the upload form re-roll the variant before submitting.
func (s *Server) handleCoverPreview(w http.ResponseWriter, r *http.Request) {
	if _, err := s.requireUser(r); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	q := r.URL.Query()
	preview := store.Book{
		// ID is mixed into the seed; for previews we use the salt as the
		// stable identifier so the same (title, salt) reproduces the same
		// design regardless of when it's rendered.
		ID:     "preview",
		Title:  q.Get("title"),
		Author: q.Get("author"),
		Format: q.Get("format"),
	}
	svg := generateCoverSVGWithSalt(preview, q.Get("salt"))
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(svg))
}

func (s *Server) handleServeBookContent(w http.ResponseWriter, r *http.Request) {
	book, _, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil {
		if errors.Is(err, errNeedsLogin) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "book not found"})
		return
	}
	contentType := safeMIME(book.MIMEType, filepath.Ext(book.OriginalFilename))
	s.serveBackendObject(w, r, book.StoragePath, contentType, book.OriginalFilename)
}

func (s *Server) handleServeBookCover(w http.ResponseWriter, r *http.Request) {
	book, _, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil {
		// Don't fall back to a cacheable placeholder for access errors —
		// otherwise a pre-login cover request poisons the browser/CDN cache
		// and the real cover keeps showing as a placeholder after auth.
		status := http.StatusNotFound
		if errors.Is(err, errNeedsLogin) {
			status = http.StatusUnauthorized
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, map[string]any{"error": "cover not available"})
		return
	}
	if book.CoverPath == "" {
		writeGeneratedCover(w, book)
		return
	}
	if _, exists, _ := s.storage.Stat(r.Context(), book.CoverPath); !exists {
		writeGeneratedCover(w, book)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	contentType := mime.TypeByExtension(filepath.Ext(book.CoverPath))
	s.serveBackendObject(w, r, book.CoverPath, contentType, "")
}

func writeGeneratedCover(w http.ResponseWriter, book store.Book) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(generateCoverSVG(book)))
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	progress, err := s.store.GetProgress(r.Context(), user.ID, book.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"progress": nil})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load progress"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": progress})
}

func (s *Server) handlePutProgress(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Locator         string  `json:"locator"`
		ProgressPercent float64 `json:"progressPercent"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Locator == "" {
		req.Locator = "start"
	}
	pct := req.ProgressPercent
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	progress := store.ReadingProgress{
		UserID:          user.ID,
		BookID:          book.ID,
		Locator:         req.Locator,
		ProgressPercent: pct,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := s.store.UpsertProgress(r.Context(), progress); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save progress"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": progress})
}

func (s *Server) handleGetHighlights(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	items, err := s.store.ListHighlights(r.Context(), user.ID, book.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load highlights"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateHighlight(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Locator      string `json:"locator"`
		SelectedText string `json:"selectedText"`
		Color        string `json:"color"`
		Note         string `json:"note"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SelectedText) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "selectedText is required"})
		return
	}
	if req.Locator == "" {
		req.Locator = "start"
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "amber"
	}
	if !validHighlightColor(color) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid color"})
		return
	}
	req.Color = color
	highlight := store.Highlight{
		ID:           randomID("hl"),
		UserID:       user.ID,
		BookID:       book.ID,
		Locator:      req.Locator,
		SelectedText: req.SelectedText,
		Color:        req.Color,
		Note:         req.Note,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.store.CreateHighlight(r.Context(), highlight); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save highlight"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"highlight": highlight})
}

func (s *Server) handleDownloadBook(w http.ResponseWriter, r *http.Request) {
	book, _, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, errNeedsLogin) {
			status = http.StatusUnauthorized
		}
		writeJSON(w, status, map[string]any{"error": "book not found"})
		return
	}
	// When the source was converted (e.g. md/pdf → epub), OriginalFilename
	// still carries the source extension. Align the served filename with the
	// stored format so downloads don't claim ".pdf" while serving epub bytes.
	filename := downloadFilename(book.OriginalFilename, book.Format)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	s.serveBackendObject(w, r, book.StoragePath, safeMIME(book.MIMEType, filepath.Ext(filename)), filename)
}

// downloadFilename returns the original filename when its extension matches
// the stored format, or rewrites the extension to match the format when the
// source was converted at upload (md/pdf → epub).
func downloadFilename(original, format string) string {
	if original == "" {
		return original
	}
	wanted := "." + strings.ToLower(strings.TrimSpace(format))
	if wanted == "." {
		return original
	}
	have := strings.ToLower(filepath.Ext(original))
	if have == wanted {
		return original
	}
	return strings.TrimSuffix(original, filepath.Ext(original)) + wanted
}

func (s *Server) handleDownloadConvertedEPUB(w http.ResponseWriter, r *http.Request) {
	book, _, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || book.DerivedEPUBPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "converted epub not found"})
		return
	}
	filename := strings.TrimSuffix(book.OriginalFilename, filepath.Ext(book.OriginalFilename)) + ".epub"
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	s.serveBackendObject(w, r, book.DerivedEPUBPath, "application/epub+zip", filename)
}

func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), true)
	if err != nil || user == nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, errNeedsLogin) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "book not found"})
		return
	}
	if err := s.store.DeleteBook(r.Context(), user.ID, book.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete book"})
		return
	}
	// Best-effort: remove every object under this book's storage prefix.
	prefix := keyForBook(book.UserID, book.ID, "")
	prefix = strings.TrimSuffix(prefix, "/")
	if err := s.storage.DeletePrefix(r.Context(), prefix); err != nil {
		log.Printf("delete book %s: storage cleanup under %q failed: %v", book.ID, prefix, err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteHighlight(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	if err := s.store.DeleteHighlight(r.Context(), user.ID, r.PathValue("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "highlight not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

const csrfCookieName = "moco_csrf"

func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		if r.TLS != nil || s.cfg.SecureCookies {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withCSRFCookie(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, tokenMissing := s.ensureCSRFCookie(w, r)
		if !isSafeMethod(r.Method) {
			header := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
			if tokenMissing || header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(token)) != 1 {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "csrf token mismatch"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return cookie.Value, false
	}
	token, err := randomToken()
	if err != nil {
		http.Error(w, "csrf token generation failed", http.StatusInternalServerError)
		return "", true
	}
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
	return token, true
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "json encoding failed", http.StatusInternalServerError)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json payload"})
		return false
	}
	return true
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(14 * 24 * time.Hour)
	if err := s.store.CreateSession(r.Context(), randomID("sess"), userID, token, expiresAt, time.Now().UTC()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
		MaxAge:   int((14 * 24 * time.Hour).Seconds()),
	})
	return nil
}

func (s *Server) requireUser(r *http.Request) (store.User, error) {
	user, err := s.currentUser(r)
	if err != nil || user == nil {
		return store.User{}, errNeedsLogin
	}
	return *user, nil
}

var errNeedsLogin = errors.New("authentication required")

func (s *Server) currentUser(r *http.Request) (*store.User, error) {
	token, err := s.sessionToken(r)
	if err != nil {
		return nil, err
	}
	session, err := s.store.GetSession(r.Context(), token)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, errors.New("session expired")
	}
	user, err := s.store.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Server) resolveBookAccess(r *http.Request, id string, requireOwner bool) (store.Book, *store.User, error) {
	book, err := s.store.GetBookAny(r.Context(), id)
	if err != nil {
		return store.Book{}, nil, err
	}
	user, _ := s.currentUser(r)
	if user != nil && user.ID == book.UserID {
		return book, user, nil
	}
	if requireOwner {
		if user == nil {
			return store.Book{}, nil, errNeedsLogin
		}
		return store.Book{}, user, store.ErrNotFound
	}
	if book.Visibility == "public" {
		return book, user, nil
	}
	// Private books may have been shared explicitly with this user.
	if user != nil {
		shared, _ := s.store.IsBookSharedWith(r.Context(), book.ID, user.ID)
		if shared {
			return book, user, nil
		}
	}
	if user == nil {
		return store.Book{}, nil, errNeedsLogin
	}
	return store.Book{}, user, store.ErrNotFound
}

func (s *Server) sessionToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(s.cfg.CookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func randomID(prefix string) string {
	token, err := randomToken()
	if err != nil {
		panic(err)
	}
	return prefix + "_" + token[:20]
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func formatFromExt(ext string) string {
	switch ext {
	case ".pdf":
		return "pdf"
	case ".epub":
		return "epub"
	case ".md", ".markdown":
		return "md"
	default:
		return ""
	}
}

func safeMIME(given, ext string) string {
	normalized := strings.TrimSpace(strings.ToLower(given))
	if strings.Contains(normalized, "/") && normalized != "application/octet-stream" && normalized != "binary/octet-stream" {
		return given
	}
	if guessed := mime.TypeByExtension(ext); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
}

// isFragmentRequest reports whether the client wants only the inner content
// fragment (used by AJAX-driven filter forms) rather than the full page.
func isFragmentRequest(r *http.Request) bool {
	return r.URL.Query().Get("fragment") == "1" || r.Header.Get("X-Fragment") == "1"
}

// tryExtractCover pulls a cover image out of the stored book file when
// possible. EPUB covers come from the OPF manifest; PDF covers are extracted
// via Calibre or mutool. Returns image bytes + extension on success.
func tryExtractCover(path, format string) ([]byte, string, bool) {
	switch format {
	case "epub":
		if data, ext, ok := epub.ExtractEPUBCover(path); ok {
			return data, ext, true
		}
	case "pdf":
		if data, ok := extractPDFCoverPNG(path); ok {
			return data, ".png", true
		}
	}
	return nil, "", false
}

// extractPDFCoverPNG renders page 1 of a PDF as PNG. Prefers mutool (fast),
// falls back to nothing if not available. Logs the reason on failure so a
// deploy without mutool installed is visible in server logs (otherwise the
// upload silently falls back to a generated SVG and users wonder why their
// PDF cover wasn't picked up).
func extractPDFCoverPNG(pdfPath string) ([]byte, bool) {
	bin, err := exec.LookPath("mutool")
	if err != nil {
		log.Printf("pdf cover extract: mutool not on PATH — install mupdf-tools to enable extraction (%v)", err)
		return nil, false
	}
	tmp, err := os.CreateTemp("", "moco-cover-*.png")
	if err != nil {
		log.Printf("pdf cover extract: tempfile failed: %v", err)
		return nil, false
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command(bin, "draw", "-r", "120", "-o", tmpPath, pdfPath, "1")
	if err := cmd.Run(); err != nil {
		log.Printf("pdf cover extract: mutool draw failed for %q: %v", pdfPath, err)
		return nil, false
	}
	body, err := os.ReadFile(tmpPath)
	if err != nil {
		log.Printf("pdf cover extract: read of rendered PNG failed: %v", err)
		return nil, false
	}
	return body, true
}

// bookJSONLD returns a schema.org Book JSON-LD payload for the reader page.
// Used for richer search results and Twitter/LinkedIn previews.
func bookJSONLD(book store.Book, url string) string {
	type bookLD struct {
		Context  string `json:"@context"`
		Type     string `json:"@type"`
		Name     string `json:"name"`
		Author   string `json:"author,omitempty"`
		BookEdit string `json:"bookFormat,omitempty"`
		URL      string `json:"url,omitempty"`
	}
	bf := ""
	switch book.Format {
	case "epub", "pdf":
		bf = "https://schema.org/EBook"
	case "md":
		bf = "https://schema.org/EBook"
	}
	payload := bookLD{
		Context:  "https://schema.org",
		Type:     "Book",
		Name:     book.Title,
		Author:   book.Author,
		BookEdit: bf,
		URL:      url,
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(out)
}

// absoluteURL returns a canonical absolute URL for the given path. Prefers
// the configured PublicURL; otherwise reconstructs from the request.
func (s *Server) absoluteURL(r *http.Request, path string) string {
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/") + path
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template rendering failed", http.StatusInternalServerError)
	}
}

func splitBooksByVisibility(books []store.Book) (privateBooks []store.Book, publicBooks []store.Book) {
	for _, book := range books {
		if book.Visibility == "public" {
			publicBooks = append(publicBooks, book)
		} else {
			privateBooks = append(privateBooks, book)
		}
	}
	return privateBooks, publicBooks
}

func takeBooks(books []store.Book, limit int) []store.Book {
	if len(books) <= limit {
		return books
	}
	return books[:limit]
}

// ----- Account management -----

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	// Pointer fields so we can tell "not provided" from "set to empty/false"
	// — lets the client patch just the bits it cares about.
	var req struct {
		DisplayName    *string `json:"displayName"`
		AnonymousOwner *bool   `json:"anonymousOwner"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "display name is required"})
			return
		}
		if len(name) > 60 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "display name is too long"})
			return
		}
		if err := s.store.UpdateDisplayName(r.Context(), user.ID, name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to update profile"})
			return
		}
		user.DisplayName = name
	}
	if req.AnonymousOwner != nil {
		if err := s.store.UpdateUserAnonymousOwner(r.Context(), user.ID, *req.AnonymousOwner); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to update profile"})
			return
		}
		user.AnonymousOwner = *req.AnonymousOwner
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	_, currentHash, err := s.store.GetUserByEmail(r.Context(), user.Email)
	if err != nil || !auth.VerifyPassword(currentHash, req.CurrentPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "current password is incorrect"})
		return
	}
	if err := auth.PasswordLooksValid(req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hashing failed"})
		return
	}
	if err := s.store.UpdatePassword(r.Context(), user.ID, newHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to change password"})
		return
	}
	// Revoke every other session so a stolen cookie loses access on password change.
	currentToken, _ := s.sessionToken(r)
	_ = s.store.DeleteUserSessionsExcept(r.Context(), user.ID, currentToken)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	_, currentHash, err := s.store.GetUserByEmail(r.Context(), user.Email)
	if err != nil || !auth.VerifyPassword(currentHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "password is incorrect"})
		return
	}
	if err := s.store.DeleteUser(r.Context(), user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to delete account"})
		return
	}
	// Best-effort: purge the user's stored objects from whatever backend is
	// configured. Goes through the storage interface so R2 / future remotes
	// get cleaned up too — the previous local-only RemoveAll left objects
	// behind on remote backends.
	if err := s.storage.DeletePrefix(r.Context(), fmt.Sprintf("books/%s/", user.ID)); err != nil {
		log.Printf("delete-account: storage cleanup for user %s failed: %v", user.ID, err)
	}
	// Drop the session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----- Tags -----

func (s *Server) handleAddTag(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Tag string `json:"tag"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	bookID := r.PathValue("id")
	// confirm ownership
	if _, err := s.store.GetBook(r.Context(), user.ID, bookID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "book not found"})
		return
	}
	if err := s.store.AddTag(r.Context(), user.ID, bookID, req.Tag); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	tags, _ := s.store.ListBookTags(r.Context(), user.ID, bookID)
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleRemoveTag(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	bookID := r.PathValue("id")
	tag := r.PathValue("tag")
	if err := s.store.RemoveTag(r.Context(), user.ID, bookID, tag); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "tag not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----- Book sharing -----

func (s *Server) handleListBookShares(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), true)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "owner only"})
		return
	}
	shares, err := s.store.ListShares(r.Context(), book.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list shares"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": shares})
}

func (s *Server) handleAddBookShare(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), true)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "owner only"})
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "email is required"})
		return
	}
	if email == strings.ToLower(user.Email) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "you already own this book"})
		return
	}
	recipient, _, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no Moco account uses that email"})
		return
	}
	if err := s.store.ShareBook(r.Context(), book.ID, recipient.ID, user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to share"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"share": map[string]any{
			"bookId":        book.ID,
			"withUserId":    recipient.ID,
			"withUserEmail": recipient.Email,
		},
	})
}

func (s *Server) handleRemoveBookShare(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), true)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "owner only"})
		return
	}
	if err := s.store.UnshareBook(r.Context(), book.ID, r.PathValue("userID")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to remove"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----- Wishlist (want-to-read) -----

func (s *Server) handleAddWishlist(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	bookID := r.PathValue("id")
	book, err := s.store.GetBookAny(r.Context(), bookID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "book not found"})
		return
	}
	// Only public books or owned books can be wishlisted.
	if book.Visibility != "public" && book.UserID != user.ID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "this book is private"})
		return
	}
	if err := s.store.AddToWishlist(r.Context(), user.ID, bookID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "wishlisted": true})
}

func (s *Server) handleRemoveWishlist(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	if err := s.store.RemoveFromWishlist(r.Context(), user.ID, r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to remove"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "wishlisted": false})
}

func (s *Server) handleListWishlist(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	books, err := s.store.ListWishlist(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load wishlist"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": books})
}

// ----- Bookmarks -----

func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	items, err := s.store.ListBookmarks(r.Context(), user.ID, book.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load bookmarks"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	book, user, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Locator string `json:"locator"`
		Label   string `json:"label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Locator) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "locator is required"})
		return
	}
	bm := store.Bookmark{
		ID:        randomID("bm"),
		UserID:    user.ID,
		BookID:    book.ID,
		Locator:   req.Locator,
		Label:     strings.TrimSpace(req.Label),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateBookmark(r.Context(), bm); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save bookmark"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"bookmark": bm})
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	if err := s.store.DeleteBookmark(r.Context(), user.ID, r.PathValue("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "bookmark not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----- Highlights extension -----

func (s *Server) handleUpdateHighlight(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	var req struct {
		Note  string `json:"note"`
		Color string `json:"color"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "amber"
	}
	if !validHighlightColor(color) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid color"})
		return
	}
	if err := s.store.UpdateHighlight(r.Context(), user.ID, r.PathValue("id"), req.Note, color); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": "highlight not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": req.Note, "color": color})
}

func validHighlightColor(c string) bool {
	switch c {
	case "amber", "sage", "rose":
		return true
	}
	return false
}

// ----- Stats page -----

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	stats, err := s.store.GetReadingStats(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}
	tagCounts, _ := s.store.ListTagCounts(r.Context(), user.ID)
	s.renderTemplate(w, "stats.html", statsPageData{
		pageData: pageData{
			Title:       "Reading stats - Moco",
			CurrentUser: &user,
			Nav:         "stats",
		},
		Stats:     stats,
		TagCounts: tagCounts,
	})
}

// ----- Settings page -----

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s.renderTemplate(w, "settings.html", pageData{
		Title:       "Settings - Moco",
		CurrentUser: &user,
		Nav:         "settings",
	})
}

// ----- PWA assets -----

func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	manifest := `{
  "name": "Moco — Personal Reader",
  "short_name": "Moco",
  "description": "A calm reader for PDF, EPUB, and Markdown.",
  "start_url": "/app",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f7f1e8",
  "theme_color": "#f6f0e7",
  "icons": [
    { "src": "/static/icon-192.svg", "sizes": "192x192", "type": "image/svg+xml" },
    { "src": "/static/icon-512.svg", "sizes": "512x512", "type": "image/svg+xml" }
  ]
}`
	_, _ = w.Write([]byte(manifest))
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	sw := `// Moco service worker — minimal "stale-while-revalidate" for static assets,
// and offline fallback for the library shell.
const CACHE = 'moco-` + AssetVersion + `';
const STATIC = ['/static/styles.css?v=` + AssetVersion + `', '/static/app.js?v=` + AssetVersion + `', '/manifest.webmanifest'];
self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE).then((c) => c.addAll(STATIC)));
  self.skipWaiting();
});
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
  );
  self.clients.claim();
});
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  // Only handle same-origin static assets and the bare app routes.
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/api/')) return;
  if (url.pathname.startsWith('/static/') || STATIC.includes(url.pathname)) {
    event.respondWith(
      caches.open(CACHE).then(async (cache) => {
        const cached = await cache.match(req);
        const fetched = fetch(req).then((res) => { if (res.ok) cache.put(req, res.clone()); return res; }).catch(() => cached);
        return cached || fetched;
      })
    );
  }
});`
	_, _ = w.Write([]byte(sw))
}

// Used by the dashboard to read sort/filter from query params.
func libraryFilterFromQuery(userID string, q url.Values) store.BookFilter {
	return store.BookFilter{
		UserID: userID,
		Tag:    q.Get("tag"),
		Format: q.Get("format"),
		Sort:   q.Get("sort"),
	}
}
