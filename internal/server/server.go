package server

import (
	"context"
	"crypto/rand"
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
	"os"
	"path/filepath"
	"strings"
	"time"

	"moco/internal/auth"
	"moco/internal/epub"
	"moco/internal/reader"
	"moco/internal/store"
)

//go:embed web/templates/*.html web/static/*
var embeddedFiles embed.FS

type Config struct {
	Addr       string
	DataDir    string
	DBPath     string
	CookieName string
}

type Server struct {
	cfg       Config
	mux       *http.ServeMux
	templates *template.Template
	store     *store.Store
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

	s := &Server{
		cfg:   cfg,
		mux:   http.NewServeMux(),
		store: dbStore,
	}

	s.templates = template.Must(template.New("").Funcs(templateFuncs()).ParseFS(embeddedFiles, "web/templates/*.html"))
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.withLogging(s.mux)
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
	s.mux.HandleFunc("GET /books/{id}", s.handleBookDetail)
	s.mux.HandleFunc("GET /books/{id}/read", s.handleReadBook)

	s.mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/v1/auth/signup", s.handleSignup)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("GET /api/v1/books", s.handleBooks)
	s.mux.HandleFunc("GET /api/v1/books/public", s.handlePublicBooks)
	s.mux.HandleFunc("POST /api/v1/books/upload", s.handleUploadBook)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/visibility", s.handleUpdateVisibility)
	s.mux.HandleFunc("GET /api/v1/books/{id}/content", s.handleServeBookContent)
	s.mux.HandleFunc("GET /api/v1/books/{id}/progress", s.handleGetProgress)
	s.mux.HandleFunc("PUT /api/v1/books/{id}/progress", s.handlePutProgress)
	s.mux.HandleFunc("GET /api/v1/books/{id}/highlights", s.handleGetHighlights)
	s.mux.HandleFunc("POST /api/v1/books/{id}/highlights", s.handleCreateHighlight)
	s.mux.HandleFunc("GET /api/v1/books/{id}/download", s.handleDownloadBook)
	s.mux.HandleFunc("GET /api/v1/books/{id}/converted.epub", s.handleDownloadConvertedEPUB)
	s.mux.HandleFunc("DELETE /api/v1/books/{id}", s.handleDeleteBook)
	s.mux.HandleFunc("DELETE /api/v1/highlights/{id}", s.handleDeleteHighlight)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	publicBooks, _ := s.store.ListPublicBooks(r.Context(), "")
	user, _ := s.currentUser(r)
	s.renderTemplate(w, "home.html", discoverPageData{
		pageData: pageData{
			Title:       "Moco",
			CurrentUser: user,
		},
		PublicBooks: takeBooks(publicBooks, 6),
	})
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	excludeUserID := ""
	if user != nil {
		excludeUserID = user.ID
	}
	publicBooks, err := s.store.ListPublicBooks(r.Context(), excludeUserID)
	if err != nil {
		http.Error(w, "failed to load public books", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "discover.html", discoverPageData{
		pageData: pageData{
			Title:       "Public Library - Moco",
			CurrentUser: user,
		},
		PublicBooks: publicBooks,
	})
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
	books, err := s.store.ListBooks(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to load dashboard", http.StatusInternalServerError)
		return
	}
	publicBooks, _ := s.store.ListPublicBooks(r.Context(), user.ID)
	privateBooks, ownPublic := splitBooksByVisibility(books)
	s.renderTemplate(w, "dashboard.html", dashboardPageData{
		pageData: pageData{
			Title:       "Library - Moco",
			CurrentUser: &user,
		},
		MyPrivateBooks: privateBooks,
		MyPublicBooks:  ownPublic,
		PublicBooks:    publicBooks,
	})
}

func (s *Server) handleBookDetail(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/books/"+r.PathValue("id")+"/read", http.StatusFound)
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

	data := readerPageData{
		pageData: pageData{
			Title:       book.Title + " - Moco",
			CurrentUser: user,
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
		}
	}

	if book.Format == "md" {
		source, err := os.ReadFile(book.StoragePath)
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
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
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
	books, err := s.store.ListPublicBooks(r.Context(), exclude)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load public books"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": books})
}

func (s *Server) handleUploadBook(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUser(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
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
	bookDir := filepath.Join(s.cfg.DataDir, "books", user.ID, bookID)
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create book directory"})
		return
	}
	storagePath := filepath.Join(bookDir, "original"+ext)
	dst, err := os.Create(storagePath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to store file"})
		return
	}
	size, copyErr := io.Copy(dst, file)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to write file"})
		return
	}

	derivedEPUBPath := ""
	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}
	if format == "md" {
		source, err := os.ReadFile(storagePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to read markdown for conversion"})
			return
		}
		epubBytes, err := epub.MarkdownToEPUB(title, author, source)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "markdown to epub conversion failed"})
			return
		}
		derivedEPUBPath = filepath.Join(bookDir, "converted.epub")
		if err := os.WriteFile(derivedEPUBPath, epubBytes, 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to persist converted epub"})
			return
		}
	}

	now := time.Now().UTC()
	book := store.Book{
		ID:               bookID,
		UserID:           user.ID,
		Title:            strings.ReplaceAll(title, "%20", " "),
		Author:           author,
		Format:           format,
		Visibility:       visibility,
		OwnerEmail:       user.Email,
		StoragePath:      storagePath,
		OriginalFilename: header.Filename,
		MIMEType:         safeMIME(header.Header.Get("Content-Type"), ext),
		DerivedEPUBPath:  derivedEPUBPath,
		FileSize:         size,
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
			"epubGenerated": derivedEPUBPath != "",
		},
	})
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
	w.Header().Set("Content-Type", safeMIME(book.MIMEType, filepath.Ext(book.OriginalFilename)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", book.OriginalFilename))
	http.ServeFile(w, r, book.StoragePath)
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
	progress := store.ReadingProgress{
		UserID:          user.ID,
		BookID:          book.ID,
		Locator:         req.Locator,
		ProgressPercent: req.ProgressPercent,
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
	if req.Color == "" {
		req.Color = "amber"
	}
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
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", book.OriginalFilename))
	http.ServeFile(w, r, book.StoragePath)
}

func (s *Server) handleDownloadConvertedEPUB(w http.ResponseWriter, r *http.Request) {
	book, _, err := s.resolveBookAccess(r, r.PathValue("id"), false)
	if err != nil || book.DerivedEPUBPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "converted epub not found"})
		return
	}
	filename := strings.TrimSuffix(book.OriginalFilename, filepath.Ext(book.OriginalFilename)) + ".epub"
	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	http.ServeFile(w, r, book.DerivedEPUBPath)
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
	_ = os.RemoveAll(filepath.Dir(book.StoragePath))
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
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
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
	if strings.Contains(given, "/") {
		return given
	}
	if guessed := mime.TypeByExtension(ext); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
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
