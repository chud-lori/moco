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
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
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
	Format           string     `json:"format"`
	Visibility       string     `json:"visibility"`
	OwnerEmail       string     `json:"ownerEmail,omitempty"`
	StoragePath      string     `json:"-"`
	OriginalFilename string     `json:"originalFilename"`
	MIMEType         string     `json:"mimeType"`
	DerivedEPUBPath  string     `json:"-"`
	FileSize         int64      `json:"fileSize"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	LastOpenedAt     *time.Time `json:"lastOpenedAt,omitempty"`
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
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, strings.ToLower(strings.TrimSpace(email)), passwordHash, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return User{}, err
	}

	user, _, err := s.GetUserByEmail(ctx, email)
	return user, err
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at
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

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, created_at, updated_at
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

func (s *Store) CreateBook(ctx context.Context, book Book) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO books (
			id, user_id, title, author, format, storage_path, cover_path, file_size,
			created_at, updated_at, last_opened_at, original_filename, mime_type, derived_epub_path, visibility
		)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?)`,
		book.ID, book.UserID, book.Title, book.Author, book.Format, book.StoragePath, book.FileSize,
		book.CreatedAt.UTC().Format(time.RFC3339Nano), book.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nullableTime(book.LastOpenedAt), book.OriginalFilename, book.MIMEType, book.DerivedEPUBPath, book.Visibility,
	)
	return err
}

func (s *Store) ListBooks(ctx context.Context, userID string) ([]Book, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path
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
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath,
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

func (s *Store) ListPublicBooks(ctx context.Context, excludeUserID string) ([]Book, error) {
	query := `
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.visibility = 'public'`
	args := []any{}
	if excludeUserID != "" {
		query += ` AND user_id != ?`
		args = append(args, excludeUserID)
	}
	query += ` ORDER BY COALESCE(b.last_opened_at, b.updated_at) DESC`

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
			&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.StoragePath,
			&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
			&book.DerivedEPUBPath,
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
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.id = ? AND b.user_id = ?`,
		id, userID,
	).Scan(
		&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.StoragePath,
		&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
		&book.DerivedEPUBPath,
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
		SELECT b.id, b.user_id, b.title, b.author, b.format, b.visibility, u.email, b.storage_path, b.file_size, b.created_at, b.updated_at,
		       b.last_opened_at, b.original_filename, b.mime_type, b.derived_epub_path
		FROM books b
		JOIN users u ON u.id = b.user_id
		WHERE b.id = ?`,
		id,
	).Scan(
		&book.ID, &book.UserID, &book.Title, &book.Author, &book.Format, &book.Visibility, &book.OwnerEmail, &book.StoragePath,
		&book.FileSize, &createdAt, &updatedAt, &lastOpened, &book.OriginalFilename, &book.MIMEType,
		&book.DerivedEPUBPath,
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

	if err := scanner.Scan(&user.ID, &user.Email, &passwordHash, &createdAt, &updatedAt); err != nil {
		return User{}, "", err
	}

	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	user.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return user, passwordHash, nil
}
