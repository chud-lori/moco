# Moco Plan

`moco` is a fast, secure reader app for people who upload and read their own `PDF`, `EPUB`, and `Markdown` files. The product should feel calm and lightweight, with strong reading ergonomics and instant resume behavior.

## Product Goals

- Support authenticated personal libraries
- Let users upload `pdf`, `epub`, and `md` files
- Provide Kindle-like chapter navigation where available
- Save highlights and allow users to revisit them
- Save last-read position precisely and resume instantly
- Keep the system simple: one VM, local disk, SQLite, low overhead
- Prioritize performance, predictable latency, and secure file ownership

## Non-Goals for MVP

- Social reading
- Public book sharing
- Collaboration
- Cloud object storage
- Multi-node deployments
- AI summaries or chat

## Core User Flows

1. User signs up or signs in
2. User uploads a book file
3. App extracts metadata and adds it to the personal library
4. User opens the book in the reader
5. User jumps through chapters or sections from the table of contents
6. User highlights passages while reading
7. App saves reading position automatically
8. User returns later and resumes from the last location

## MVP Features

### Authentication

- Email/password sign-up and sign-in
- Secure session cookies
- Password hashing with `argon2id`
- Protected routes and per-user authorization checks

### Library

- Upload `PDF`, `EPUB`, and `Markdown`
- Show cover, title, author, format, and last-opened state
- Sort by recent activity
- Remove a book from the library

### Reader

- Dedicated reading view for each format
- Table of contents / chapter navigation
- Save and restore last-read position
- Reading controls for typography, width, and theme

### Highlights

- Text selection and highlight creation
- Highlight colors
- Highlights list per book
- Jump from a highlight back to the original location
- Delete highlight

## Architecture

### Deployment Model

- Single VM
- Reverse proxy: `Caddy`
- Backend API: `Go`
- Frontend: web app, served by the same VM
- Database: `SQLite`
- File storage: local filesystem
- Background work: in-process jobs inside the backend

### Why This Shape

- `Go` gives low memory usage, strong concurrency, and easy streaming I/O
- `SQLite` is the lightest serious relational datastore for a single-node system
- Local disk is simpler and faster than external object storage for this setup
- One VM keeps operations cheap and understandable

## Suggested Tech Stack

### Backend

- `Go`
- Router: `chi` or standard `net/http`
- SQL: `database/sql` with `modernc.org/sqlite` or `mattn/go-sqlite3`
- Migrations: `goose`
- Password hashing: `argon2id`
- Session management: signed, secure cookie-backed sessions with DB session records if needed

### Frontend

- Start with `Next.js` + `TypeScript`
- Styling: `Tailwind CSS` or hand-authored CSS modules
- Use dynamic imports for heavy reader engines
- Only load format-specific readers on demand

### Reader Engines

- `pdf.js` for `PDF`
- `epub.js` for `EPUB`
- `react-markdown` plus `remark` pipeline for `Markdown`

## Data Model

### users

- `id`
- `email`
- `password_hash`
- `created_at`
- `updated_at`

### sessions

- `id`
- `user_id`
- `token_hash`
- `expires_at`
- `created_at`

### books

- `id`
- `user_id`
- `title`
- `author`
- `format`
- `storage_path`
- `cover_path`
- `file_size`
- `created_at`
- `updated_at`
- `last_opened_at`

### book_toc_entries

- `id`
- `book_id`
- `label`
- `href`
- `depth`
- `sort_order`

### reading_progress

- `id`
- `user_id`
- `book_id`
- `locator`
- `progress_percent`
- `updated_at`

### highlights

- `id`
- `user_id`
- `book_id`
- `locator`
- `selected_text`
- `color`
- `note`
- `created_at`

## Locator Strategy

Each format needs its own locator model, but the API should expose a single `locator` payload.

### EPUB

- Store `CFI` or equivalent canonical location
- Support chapter-level TOC jump using spine/nav data

### PDF

- Store page number and selection bounds
- Use embedded outline when present for chapter navigation
- Accept that PDF highlighting is the hardest format

### Markdown

- Store heading anchor and text offset
- Build TOC from heading structure

## Filesystem Layout

```text
/var/lib/moco/
  books/
    {user_id}/
      {book_id}/
        original.epub
        original.pdf
        original.md
        cover.jpg
        toc.json
        metadata.json
```

## Performance Plan

- Stream uploads directly to disk
- Validate file size and type before expensive processing
- Extract metadata once during ingest, not on every read
- Cache TOC and derived metadata on local disk
- Lazy-load `pdf.js` and `epub.js` only on reader routes
- Debounce progress writes
- Keep highlight writes small and append-like
- Use `WAL` mode in SQLite
- Add targeted indexes for library, progress, and highlights queries
- Avoid ORM overhead and generate explicit SQL queries

## SQLite Notes

- Enable `WAL` mode
- Set sane busy timeout
- Use one shared DB file on local SSD
- Back up the DB and uploaded files together
- Migrate to Postgres later only if single-node limits become real

## Security Plan

- Hash passwords with `argon2id`
- Use `HttpOnly`, `Secure`, `SameSite=Lax` cookies
- Enforce auth on every book, highlight, and progress endpoint
- Validate MIME type and extension on upload
- Set max file sizes
- Sanitize rendered Markdown
- Keep uploaded files outside public web roots
- Serve files through authorized handlers
- Rate-limit login and upload endpoints
- Log auth failures and suspicious upload attempts

## API Shape

### Auth

- `POST /api/auth/signup`
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`

### Library

- `GET /api/books`
- `POST /api/books/upload`
- `GET /api/books/:id`
- `DELETE /api/books/:id`

### Reading

- `GET /api/books/:id/toc`
- `GET /api/books/:id/progress`
- `PUT /api/books/:id/progress`

### Highlights

- `GET /api/books/:id/highlights`
- `POST /api/books/:id/highlights`
- `DELETE /api/highlights/:id`

## Delivery Phases

### Phase 1: Foundation

- Settle branding and product direction
- Scaffold frontend and backend repos
- Set up SQLite migrations
- Build auth and protected app shell

### Phase 2: Library

- Upload pipeline
- Local file storage
- Metadata extraction
- Library screen

### Phase 3: Reader Core

- EPUB reader
- Markdown reader
- TOC navigation
- Last-read persistence

### Phase 4: Highlights

- Selection model
- Highlight persistence
- Highlight panel and jump-back

### Phase 5: PDF

- PDF reader integration
- Outline-based navigation
- Basic highlight support

### Phase 6: Hardening

- Profiling
- SQLite tuning
- Error handling
- Security review
- Backup and restore strategy

## Recommended Build Order

1. Frontend mock and design system direction
2. Backend skeleton in `Go`
3. SQLite schema and migrations
4. Auth
5. Upload and library
6. EPUB reader
7. Markdown reader
8. Reading progress
9. Highlights
10. PDF support
11. Performance and security pass

## Success Criteria

`moco` MVP is successful when a user can:

- Create an account and sign in securely
- Upload supported file types
- Browse a personal library quickly
- Open a book and jump to sections reliably
- Highlight text and revisit highlights
- Close and reopen the app and resume from the last location without friction
