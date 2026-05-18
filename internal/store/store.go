package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrNotFound = errors.New("not found")

type Store struct {
	db *sql.DB
}

type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	DisplayName    string    `json:"displayName"`
	AnonymousOwner bool      `json:"anonymousOwner"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Book struct {
	ID               string     `json:"id"`
	UserID           string     `json:"userId"`
	Title            string     `json:"title"`
	Author           string     `json:"author"`
	// Description is plain text, auto-extracted from EPUB <dc:description>
	// on upload when present, and editable by the owner via the edit modal.
	// Older rows default to "" — display layer treats empty as "no description".
	Description      string     `json:"description"`
	Format           string     `json:"format"`
	Visibility       string     `json:"visibility"`
	OwnerEmail       string     `json:"ownerEmail,omitempty"`
	OwnerDisplayName string     `json:"ownerDisplayName,omitempty"`
	// AnonymousOwner is sourced from the OWNER's user setting (not the
	// book itself), populated via the JOIN on users in every book SELECT.
	// When true, templates omit the "by {owner}" byline. Toggling this in
	// account settings affects all of the user's books at once.
	AnonymousOwner   bool       `json:"anonymousOwner"`
	StoragePath      string     `json:"-"`
	OriginalFilename string     `json:"originalFilename"`
	MIMEType         string     `json:"mimeType"`
	DerivedEPUBPath  string     `json:"-"`
	CoverPath        string     `json:"-"`
	FileSize         int64      `json:"fileSize"`
	ReadingMinutes   int        `json:"readingMinutes"`
	// TotalPages is reported by PDF.js after the user opens a PDF
	// (pure-Go page extraction at upload time isn't worth the dep). 0
	// means "unknown" — older books or non-PDF formats stay at 0.
	TotalPages       int        `json:"totalPages"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	LastOpenedAt     *time.Time `json:"lastOpenedAt,omitempty"`
}

type Bookmark struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	BookID    string    `json:"bookId"`
	Locator   string    `json:"locator"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
}

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type ReadingStats struct {
	BookCount       int `json:"bookCount"`
	HighlightCount  int `json:"highlightCount"`
	BookmarkCount   int `json:"bookmarkCount"`
	BooksStarted    int `json:"booksStarted"`
	BooksFinished   int `json:"booksFinished"`
	TagCount        int `json:"tagCount"`
	ReadingMinutes  int `json:"readingMinutes"`
	HighlightsBooks int `json:"highlightsBooks"`
}

// BookFilter encapsulates dashboard list query options.
type BookFilter struct {
	UserID  string
	Tag     string // empty = all
	Format  string // empty = all
	Sort    string // "recent" | "title" | "progress" | "added"
	OnlyOwn bool   // ignored — caller already filters by user_id
}

type ReadingProgress struct {
	UserID          string    `json:"userId"`
	BookID          string    `json:"bookId"`
	Locator         string    `json:"locator"`
	ProgressPercent float64   `json:"progressPercent"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Highlight struct {
	ID           string    `json:"id"`
	UserID       string    `json:"userId"`
	BookID       string    `json:"bookId"`
	Locator      string    `json:"locator"`
	SelectedText string    `json:"selectedText"`
	Color        string    `json:"color"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"createdAt"`
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	}
	for _, stmt := range pragmas {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, err
		}
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		body, err := fs.ReadFile(migrationFiles, "migrations/"+entry.Name())
		if err != nil {
			return err
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) CreateUser(ctx context.Context, id, email, passwordHash string, now time.Time) (User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	displayName := defaultDisplayName(cleanEmail)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at, updated_at, display_name)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, cleanEmail, passwordHash, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano), displayName,
	)
	if err != nil {
		return User{}, err
	}

	user, _, err := s.GetUserByEmail(ctx, email)
	return user, err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at, display_name, anonymous_owner
		FROM users
		WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email)),
	)

	user, passwordHash, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, "", ErrNotFound
		}
		return User{}, "", err
	}

	return user, passwordHash, nil
}

// HasPassword reports whether a user has a usable password set. Returns
// false for users created via Google OAuth who never set one — those rows
// have password_hash == "". The settings page uses this to flip between
// the "Change password" and "Set a password" UI.
func (s *Store) HasPassword(ctx context.Context, userID string) (bool, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	return hash != "", nil
}

// GetUserByGoogleSub looks up a user by their Google subject identifier
// (the stable, opaque "sub" claim from Google's ID token). Returns
// ErrNotFound when no row matches — caller typically falls back to
// GetUserByEmail (auto-link) and then to creating a new user.
func (s *Store) GetUserByGoogleSub(ctx context.Context, sub string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at, display_name, anonymous_owner
		FROM users
		WHERE google_sub = ?`, sub)
	user, _, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return user, nil
}

// LinkGoogleSub records a Google identity on an existing user — used in
// the auto-link flow when a Google email already matches an existing
// password account. Idempotent: re-linking the same sub is a no-op.
func (s *Store) LinkGoogleSub(ctx context.Context, userID, sub string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET google_sub = ?, updated_at = ? WHERE id = ?`,
		sub, time.Now().UTC().Format(time.RFC3339Nano), userID,
	)
	return err
}

// CreateUserFromGoogle creates a new user with no password (password_hash
// is set to empty string, which VerifyPassword always rejects) and an
// initial display name from the Google profile. The google_sub column is
// populated atomically so subsequent logins find the user via
// GetUserByGoogleSub.
func (s *Store) CreateUserFromGoogle(ctx context.Context, id, email, displayName, sub string, now time.Time) (User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = defaultDisplayName(cleanEmail)
	}
	ts := now.UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at, updated_at, display_name, google_sub)
		VALUES (?, ?, '', ?, ?, ?, ?)`,
		id, cleanEmail, ts, ts, name, sub,
	)
	if err != nil {
		return User{}, err
	}
	user, _, err := s.GetUserByEmail(ctx, cleanEmail)
	return user, err
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at, display_name, anonymous_owner
		FROM users
		WHERE id = ?`,
		id,
	)

	user, _, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}

	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, id, userID, rawToken string, expiresAt, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, userID, hashToken(rawToken), expiresAt.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetSession(ctx context.Context, rawToken string) (Session, error) {
	var session Session
	var expiresAt string
	var createdAt string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM sessions
		WHERE token_hash = ?`,
		hashToken(rawToken),
	).Scan(&session.ID, &session.UserID, &session.TokenHash, &expiresAt, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}

	session.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	session.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, rawToken string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(rawToken))
	return err
}

// DeleteUserSessionsExcept removes every session for the given user, leaving
// only the session matching keepRawToken (pass empty string to revoke all).
// Used after password change so existing sessions cannot keep authenticating.
func (s *Store) DeleteUserSessionsExcept(ctx context.Context, userID, keepRawToken string) error {
	if keepRawToken == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`,
		userID, hashToken(keepRawToken))
	return err
}

func (s *Store) CreateBook(ctx context.Context, book Book) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO books (
			id, user_id, title, author, format, storage_path, cover_path, file_size,
			created_at, updated_at, last_opened_at, original_filename, mime_type, derived_epub_path, visibility, reading_minutes, description
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID, book.UserID, book.Title, book.Author, book.Format, book.StoragePath, book.CoverPath, book.FileSize,
		book.CreatedAt.UTC().Format(time.RFC3339Nano), book.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nullableTime(book.LastOpenedAt), book.OriginalFilename, book.MIMEType, book.DerivedEPUBPath, book.Visibility, book.ReadingMinutes, book.Description,
	)
	return err
}

// UpdateUserAnonymousOwner toggles the user-level "hide my name on book
// bylines" setting. Affects all books the user has uploaded — every book
// SELECT joins users.anonymous_owner into Book.AnonymousOwner.
func (s *Store) UpdateUserAnonymousOwner(ctx context.Context, userID string, anonymous bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET anonymous_owner = ?, updated_at = ? WHERE id = ?`,
		anonymous, time.Now().UTC().Format(time.RFC3339Nano), userID)
	return err
}

// UpdateBookTotalPages persists the PDF page count + a derived reading-
// time estimate. Called from the reader once PDF.js reports numPages on
// open. Idempotent — safe to call on every reader load. Only updates if
// the values actually changed (avoids touching updated_at unnecessarily).
func (s *Store) UpdateBookTotalPages(ctx context.Context, userID, bookID string, totalPages, readingMinutes int) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET total_pages = ?, reading_minutes = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
		  AND (total_pages != ? OR reading_minutes != ?)`,
		totalPages, readingMinutes, time.Now().UTC().Format(time.RFC3339Nano),
		bookID, userID,
		totalPages, readingMinutes)
	if err != nil {
		return err
	}
	_, _ = res.RowsAffected() // 0 rows is fine — value already matched.
	return nil
}

func (s *Store) ListBooks(ctx context.Context, userID string) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.user_id = ?
		ORDER BY COALESCE(b.last_opened_at, b.updated_at) DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		var createdAt string
		var updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}

	return books, rows.Err()
}

func (s *Store) ListPublicBooks(ctx context.Context, excludeUserID, search, sort, format string) ([]Book, error) {
	query := `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.visibility = 'public'`
	args := []any{}
	if excludeUserID != "" {
		query += ` AND user_id != ?`
		args = append(args, excludeUserID)
	}
	if search = strings.TrimSpace(search); search != "" {
		// Match against title, author, description (so a phrase from the
		// blurb finds the book even when the title slips your mind), and
		// owner display name for non-anonymous uploaders (so a viewer
		// searching for a person finds their public shelf without
		// learning the anonymity rules).
		query += ` AND (lower(b.title) LIKE ? OR lower(b.author) LIKE ? OR lower(COALESCE(b.description, '')) LIKE ? OR (u.anonymous_owner = 0 AND lower(COALESCE(u.display_name, '')) LIKE ?))`
		needle := "%" + strings.ToLower(search) + "%"
		args = append(args, needle, needle, needle, needle)
	}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pdf", "epub", "md":
		query += ` AND b.format = ?`
		args = append(args, strings.ToLower(strings.TrimSpace(format)))
	}
	switch sort {
	case "title":
		query += ` ORDER BY b.title COLLATE NOCASE ASC`
	default:
		query += ` ORDER BY COALESCE(b.last_opened_at, b.updated_at) DESC`
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		var createdAt string
		var updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *Store) GetBook(ctx context.Context, userID, id string) (Book, error) {
	var book Book
	var createdAt string
	var updatedAt string
	var lastOpened sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.id = ? AND b.user_id = ?`,
		id, userID,
	).Scan(
		&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
		&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
		&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Book{}, ErrNotFound
		}
		return Book{}, err
	}

	book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastOpened.Valid {
		t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
		book.LastOpenedAt = &t
	}

	return book, nil
}

func (s *Store) GetBookAny(ctx context.Context, id string) (Book, error) {
	var book Book
	var createdAt string
	var updatedAt string
	var lastOpened sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.id = ?`,
		id,
	).Scan(
		&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
		&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
		&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Book{}, ErrNotFound
		}
		return Book{}, err
	}
	book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if lastOpened.Valid {
		t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
		book.LastOpenedAt = &t
	}
	return book, nil
}

func (s *Store) DeleteBook(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM books WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateBookVisibility(ctx context.Context, userID, id, visibility string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET visibility = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		visibility, time.Now().UTC().Format(time.RFC3339Nano), id, userID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateBookMetadata rewrites the editable user-facing fields (title, author).
// Owner-scoped: returns ErrNotFound if the book doesn't exist or the caller
// isn't the owner.
func (s *Store) UpdateBookMetadata(ctx context.Context, userID, id, title, author, description string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET title = ?, author = ?, description = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		title, author, description, time.Now().UTC().Format(time.RFC3339Nano), id, userID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateBookCoverPath replaces the cover_path column on a single book. Pass an
// empty string to clear it (handler then falls back to the on-the-fly
// generator). Owner-scoped.
func (s *Store) UpdateBookCoverPath(ctx context.Context, userID, id, coverPath string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE books
		SET cover_path = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		coverPath, time.Now().UTC().Format(time.RFC3339Nano), id, userID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertProgress(ctx context.Context, progress ReadingProgress) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reading_progress (id, user_id, book_id, locator, progress_percent, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, book_id) DO UPDATE SET
			locator = excluded.locator,
			progress_percent = excluded.progress_percent,
			updated_at = excluded.updated_at`,
		"prog_"+progress.UserID+"_"+progress.BookID,
		progress.UserID,
		progress.BookID,
		progress.Locator,
		progress.ProgressPercent,
		progress.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE books
		SET last_opened_at = ?, updated_at = ?
		WHERE id = ? AND user_id = ?`,
		progress.UpdatedAt.UTC().Format(time.RFC3339Nano),
		progress.UpdatedAt.UTC().Format(time.RFC3339Nano),
		progress.BookID,
		progress.UserID,
	)
	return err
}

func (s *Store) GetProgress(ctx context.Context, userID, bookID string) (ReadingProgress, error) {
	var progress ReadingProgress
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, book_id, locator, progress_percent, updated_at
		FROM reading_progress
		WHERE user_id = ? AND book_id = ?`,
		userID, bookID,
	).Scan(&progress.UserID, &progress.BookID, &progress.Locator, &progress.ProgressPercent, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReadingProgress{}, ErrNotFound
		}
		return ReadingProgress{}, err
	}
	progress.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return progress, nil
}

// ListInProgressBooks returns books the user has actively started reading
// (progress > 0.5% and < 95%), ordered by most-recent activity. Used by
// the dashboard's top "Continue reading" section to surface in-flight
// reads across every shelf (own, public, wishlisted, shared) in one
// curated row rather than leaving the user to hunt for them. Books that
// have since become inaccessible (private + not owned, etc.) are filtered
// out by the join + visibility predicate.
func (s *Store) ListInProgressBooks(ctx context.Context, userID string, limit int) ([]Book, error) {
	if userID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 8
	}
	q := `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = ?
		WHERE rp.progress_percent > 0.5 AND rp.progress_percent < 95
		  AND (
		        b.user_id = ?
		     OR b.visibility = 'public'
		     OR b.id IN (SELECT book_id FROM book_shares WHERE shared_with_user_id = ?)
		  )
		ORDER BY rp.updated_at DESC
		LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, userID, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var book Book
		var createdAt, updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// ProgressByBookIDs returns progress_percent keyed by book ID for the given
// user. Books with no row in reading_progress are omitted — callers should
// treat a missing key as "unread". Used by list pages (dashboard / discover)
// to render "Continue reading" affordances + progress bars on covers
// without hitting reading_progress once per card.
func (s *Store) ProgressByBookIDs(ctx context.Context, userID string, bookIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if userID == "" || len(bookIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(bookIDs)), ",")
	q := `SELECT book_id, progress_percent FROM reading_progress
	      WHERE user_id = ? AND book_id IN (` + placeholders + `)`
	args := make([]any, 0, len(bookIDs)+1)
	args = append(args, userID)
	for _, id := range bookIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var bid string
		var pct float64
		if err := rows.Scan(&bid, &pct); err != nil {
			return nil, err
		}
		out[bid] = pct
	}
	return out, rows.Err()
}

func (s *Store) CreateHighlight(ctx context.Context, highlight Highlight) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO highlights (id, user_id, book_id, locator, selected_text, color, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		highlight.ID,
		highlight.UserID,
		highlight.BookID,
		highlight.Locator,
		highlight.SelectedText,
		highlight.Color,
		highlight.Note,
		highlight.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) ListHighlights(ctx context.Context, userID, bookID string) ([]Highlight, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, book_id, locator, selected_text, color, note, created_at
		FROM highlights
		WHERE user_id = ? AND book_id = ?
		ORDER BY created_at DESC`,
		userID, bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Highlight
	for rows.Next() {
		var item Highlight
		var createdAt string
		if err := rows.Scan(&item.ID, &item.UserID, &item.BookID, &item.Locator, &item.SelectedText, &item.Color, &item.Note, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

// HighlightWithBook is a highlight enriched with the book it was made on,
// for the cross-book Quotes page.
type HighlightWithBook struct {
	Highlight
	BookTitle  string `json:"bookTitle"`
	BookAuthor string `json:"bookAuthor"`
	BookFormat string `json:"bookFormat"`
}

func (s *Store) ListAllHighlights(ctx context.Context, userID string) ([]HighlightWithBook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT h.id, h.user_id, h.book_id, h.locator, h.selected_text, h.color, h.note, h.created_at,
		       b.title, b.author, b.format
		FROM highlights h
		JOIN books b ON b.id = h.book_id
		WHERE h.user_id = ?
		ORDER BY h.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []HighlightWithBook
	for rows.Next() {
		var item HighlightWithBook
		var createdAt string
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.BookID, &item.Locator, &item.SelectedText, &item.Color, &item.Note, &createdAt,
			&item.BookTitle, &item.BookAuthor, &item.BookFormat,
		); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteHighlight(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM highlights WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func defaultDisplayName(email string) string {
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return ""
}

// ----- Account management -----

func (s *Store) UpdateDisplayName(ctx context.Context, userID, name string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET display_name = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(name), now, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdatePassword(ctx context.Context, userID, newHash string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		newHash, now, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	// CASCADE handles sessions/books/highlights/bookmarks/book_tags.
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ----- Tags -----

func (s *Store) AddTag(ctx context.Context, userID, bookID, tag string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return errors.New("tag is empty")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO book_tags (user_id, book_id, tag, created_at) VALUES (?, ?, ?, ?)`,
		userID, bookID, tag, now)
	return err
}

func (s *Store) RemoveTag(ctx context.Context, userID, bookID, tag string) error {
	tag = normalizeTag(tag)
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM book_tags WHERE user_id = ? AND book_id = ? AND tag = ?`,
		userID, bookID, tag)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListBookTags(ctx context.Context, userID, bookID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag FROM book_tags WHERE user_id = ? AND book_id = ? ORDER BY tag`,
		userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListTagCounts(ctx context.Context, userID string) ([]TagCount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tag, COUNT(*) FROM book_tags WHERE user_id = ? GROUP BY tag ORDER BY tag`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagCount
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Tag, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// AttachTagsToBooks fetches the tag list for each book and assigns it via the
// callback. It runs one query (SELECT book_id, tag FROM book_tags WHERE
// user_id IN ...) regardless of how many books are passed.
func (s *Store) AttachTagsToBooks(ctx context.Context, userID string, books []Book) (map[string][]string, error) {
	if len(books) == 0 {
		return map[string][]string{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id, tag FROM book_tags WHERE user_id = ? ORDER BY tag`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string, len(books))
	for rows.Next() {
		var bookID, tag string
		if err := rows.Scan(&bookID, &tag); err != nil {
			return nil, err
		}
		out[bookID] = append(out[bookID], tag)
	}
	return out, rows.Err()
}

func normalizeTag(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	// collapse internal whitespace into single spaces
	t = strings.Join(strings.Fields(t), " ")
	if len(t) > 32 {
		t = t[:32]
	}
	return t
}

// ----- Bookmarks -----

func (s *Store) CreateBookmark(ctx context.Context, b Bookmark) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bookmarks (id, user_id, book_id, locator, label, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.UserID, b.BookID, b.Locator, b.Label,
		b.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListBookmarks(ctx context.Context, userID, bookID string) ([]Bookmark, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, book_id, locator, label, created_at
		 FROM bookmarks
		 WHERE user_id = ? AND book_id = ?
		 ORDER BY created_at DESC`,
		userID, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bookmark
	for rows.Next() {
		var b Bookmark
		var createdAt string
		if err := rows.Scan(&b.ID, &b.UserID, &b.BookID, &b.Locator, &b.Label, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBookmark(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM bookmarks WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ----- Highlights extensions -----

func (s *Store) UpdateHighlight(ctx context.Context, userID, id, note, color string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE highlights SET note = ?, color = ? WHERE id = ? AND user_id = ?`,
		note, color, id, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ----- Reading stats -----

func (s *Store) GetReadingStats(ctx context.Context, userID string) (ReadingStats, error) {
	var stats ReadingStats
	row := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM books WHERE user_id = ?),
			(SELECT COUNT(*) FROM highlights WHERE user_id = ?),
			(SELECT COUNT(*) FROM bookmarks WHERE user_id = ?),
			(SELECT COUNT(*) FROM reading_progress WHERE user_id = ? AND progress_percent > 0),
			(SELECT COUNT(*) FROM reading_progress WHERE user_id = ? AND progress_percent >= 95),
			(SELECT COUNT(DISTINCT tag) FROM book_tags WHERE user_id = ?),
			(SELECT COALESCE(SUM(b.reading_minutes), 0) FROM books b
			   JOIN reading_progress rp ON rp.book_id = b.id AND rp.user_id = b.user_id
			   WHERE b.user_id = ?),
			(SELECT COUNT(DISTINCT book_id) FROM highlights WHERE user_id = ?)
	`, userID, userID, userID, userID, userID, userID, userID, userID)
	if err := row.Scan(&stats.BookCount, &stats.HighlightCount, &stats.BookmarkCount,
		&stats.BooksStarted, &stats.BooksFinished, &stats.TagCount, &stats.ReadingMinutes,
		&stats.HighlightsBooks); err != nil {
		return ReadingStats{}, err
	}
	return stats, nil
}

// ----- Library list with sort/filter -----

func (s *Store) ListBooksFiltered(ctx context.Context, opts BookFilter) ([]Book, error) {
	q := `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.user_id = ?`
	args := []any{opts.UserID}
	if opts.Format != "" {
		q += ` AND b.format = ?`
		args = append(args, opts.Format)
	}
	if opts.Tag != "" {
		q += ` AND b.id IN (SELECT book_id FROM book_tags WHERE user_id = ? AND tag = ?)`
		args = append(args, opts.UserID, normalizeTag(opts.Tag))
	}
	switch opts.Sort {
	case "title":
		q += ` ORDER BY b.title COLLATE NOCASE ASC`
	case "added":
		q += ` ORDER BY b.created_at DESC`
	case "progress":
		q += ` ORDER BY (SELECT progress_percent FROM reading_progress WHERE user_id = b.user_id AND book_id = b.id) DESC NULLS LAST`
	default:
		q += ` ORDER BY COALESCE(b.last_opened_at, b.updated_at) DESC`
	}

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var book Book
		var createdAt string
		var updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// SearchHighlights returns the user's highlights with book info, optionally
// filtered by free-text query (case-insensitive substring on text + book title)
// and/or a specific book ID.
func (s *Store) SearchHighlights(ctx context.Context, userID, query, bookID string) ([]HighlightWithBook, error) {
	q := `
		SELECT h.id, h.user_id, h.book_id, h.locator, h.selected_text, h.color, h.note, h.created_at,
		       b.title, b.author, b.format
		FROM highlights h
		JOIN books b ON b.id = h.book_id
		WHERE h.user_id = ?`
	args := []any{userID}
	if bookID != "" {
		q += ` AND h.book_id = ?`
		args = append(args, bookID)
	}
	if query = strings.TrimSpace(query); query != "" {
		q += ` AND (lower(h.selected_text) LIKE ? OR lower(h.note) LIKE ? OR lower(b.title) LIKE ?)`
		needle := "%" + strings.ToLower(query) + "%"
		args = append(args, needle, needle, needle)
	}
	q += ` ORDER BY h.created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []HighlightWithBook
	for rows.Next() {
		var item HighlightWithBook
		var createdAt string
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.BookID, &item.Locator, &item.SelectedText, &item.Color, &item.Note, &createdAt,
			&item.BookTitle, &item.BookAuthor, &item.BookFormat,
		); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

// AllBooksAdmin returns every book in the system. Used only by the storage
// migration command — not exposed via any HTTP route.
func (s *Store) AllBooksAdmin(ctx context.Context) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path,
		       b.file_size, b.created_at, b.updated_at, b.last_opened_at, b.original_filename,
		       b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM books b
		JOIN users u ON u.id = b.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var book Book
		var createdAt, updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility,
			&book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath, &book.FileSize, &createdAt, &updatedAt, &lastOpened,
			&book.OriginalFilename, &book.MIMEType, &book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// UpdateBookPaths rewrites the three path columns on a book. Used by the
// storage-migration command after files have been pushed to the new backend.
func (s *Store) UpdateBookPaths(ctx context.Context, bookID, storagePath, derivedEPUBPath, coverPath string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE books SET storage_path = ?, derived_epub_path = ?, cover_path = ?, updated_at = ? WHERE id = ?`,
		storagePath, derivedEPUBPath, coverPath, time.Now().UTC().Format(time.RFC3339Nano), bookID)
	return err
}

// ----- Book shares -----

type Share struct {
	BookID         string    `json:"bookId"`
	WithUserID     string    `json:"withUserId"`
	WithUserEmail  string    `json:"withUserEmail"`
	ByUserID       string    `json:"byUserId"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ShareBook grants the recipient access to read the book. The caller is
// expected to have validated ownership beforehand. Idempotent.
func (s *Store) ShareBook(ctx context.Context, bookID, recipientUserID, ownerUserID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO book_shares (book_id, shared_with_user_id, shared_by_user_id, created_at)
		 VALUES (?, ?, ?, ?)`,
		bookID, recipientUserID, ownerUserID, now)
	return err
}

func (s *Store) UnshareBook(ctx context.Context, bookID, recipientUserID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM book_shares WHERE book_id = ? AND shared_with_user_id = ?`,
		bookID, recipientUserID)
	return err
}

// ListShares returns the recipient list for a book (owner-facing view).
func (s *Store) ListShares(ctx context.Context, bookID string) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.book_id, s.shared_with_user_id, u.email, s.shared_by_user_id, s.created_at
		FROM book_shares s
		JOIN users u ON u.id = s.shared_with_user_id
		WHERE s.book_id = ?
		ORDER BY s.created_at DESC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Share
	for rows.Next() {
		var sh Share
		var createdAt string
		if err := rows.Scan(&sh.BookID, &sh.WithUserID, &sh.WithUserEmail, &sh.ByUserID, &createdAt); err != nil {
			return nil, err
		}
		sh.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, sh)
	}
	return out, rows.Err()
}

// IsBookSharedWith reports whether userID has access to bookID via a share.
func (s *Store) IsBookSharedWith(ctx context.Context, bookID, userID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM book_shares WHERE book_id = ? AND shared_with_user_id = ? LIMIT 1`,
		bookID, userID).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListBooksSharedWithUser returns all books that other users have shared
// with the given recipient. Used in the dashboard "Shared with you" section.
func (s *Store) ListBooksSharedWithUser(ctx context.Context, userID string) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path,
		       b.file_size, b.created_at, b.updated_at, b.last_opened_at, b.original_filename,
		       b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM book_shares s
		JOIN books b ON b.id = s.book_id
		JOIN users u ON u.id = b.user_id
		WHERE s.shared_with_user_id = ?
		ORDER BY s.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var book Book
		var createdAt, updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility,
			&book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath, &book.FileSize, &createdAt, &updatedAt, &lastOpened,
			&book.OriginalFilename, &book.MIMEType, &book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// ----- Wishlist (want-to-read) -----

// AddToWishlist saves a book to the user's wishlist. Idempotent.
func (s *Store) AddToWishlist(ctx context.Context, userID, bookID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO wishlist (user_id, book_id, created_at) VALUES (?, ?, ?)`,
		userID, bookID, now)
	return err
}

// RemoveFromWishlist removes a book from the user's wishlist.
func (s *Store) RemoveFromWishlist(ctx context.Context, userID, bookID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM wishlist WHERE user_id = ? AND book_id = ?`, userID, bookID)
	return err
}

// IsInWishlist reports whether the user has added a book to their wishlist.
func (s *Store) IsInWishlist(ctx context.Context, userID, bookID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM wishlist WHERE user_id = ? AND book_id = ? LIMIT 1`, userID, bookID,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListWishlist returns the user's wishlist as a slice of Book records (joins
// across the books + users tables). Skips books that have since been deleted.
func (s *Store) ListWishlist(ctx context.Context, userID string) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, u.display_name, b.storage_path,
		       b.file_size, b.created_at, b.updated_at, b.last_opened_at, b.original_filename,
		       b.mime_type, b.derived_epub_path, b.reading_minutes, b.cover_path, u.anonymous_owner, b.total_pages, b.description
		FROM wishlist w
		JOIN books b ON b.id = w.book_id
		JOIN users u ON u.id = b.user_id
		WHERE w.user_id = ?
		ORDER BY w.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		var createdAt, updatedAt string
		var lastOpened sql.NullString
		if err := rows.Scan(
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility,
			&book.OwnerEmail, &book.OwnerDisplayName, &book.StoragePath, &book.FileSize, &createdAt, &updatedAt, &lastOpened,
			&book.OriginalFilename, &book.MIMEType, &book.DerivedEPUBPath, &book.ReadingMinutes, &book.CoverPath, &book.AnonymousOwner, &book.TotalPages, &book.Description,
		); err != nil {
			return nil, err
		}
		book.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		book.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if lastOpened.Valid {
			t, _ := time.Parse(time.RFC3339Nano, lastOpened.String)
			book.LastOpenedAt = &t
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// WishlistedBookIDs returns the set of book IDs that the user has wishlisted,
// scoped to a specific list of candidate IDs (used to mark cards in lists).
func (s *Store) WishlistedBookIDs(ctx context.Context, userID string, candidates []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(candidates) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(candidates))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(candidates)+1)
	args = append(args, userID)
	for _, id := range candidates {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT book_id FROM wishlist WHERE user_id = ? AND book_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func scanUser(scanner interface {
	Scan(dest ...any) error
}) (User, string, error) {
	var user User
	var passwordHash string
	var createdAt string
	var updatedAt string
	var displayName sql.NullString

	if err := scanner.Scan(&user.ID, &user.Email, &passwordHash, &createdAt, &updatedAt, &displayName, &user.AnonymousOwner); err != nil {
		return User{}, "", err
	}

	if displayName.Valid {
		user.DisplayName = displayName.String
	}
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	user.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return user, passwordHash, nil
}
