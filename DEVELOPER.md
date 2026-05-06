# Moco — Developer Guide

Everything an engineer needs to run, change, and ship Moco. Product-side pitch and feature list live in the [README](./README.md).

---

## Table of contents

1. [Architecture overview](#architecture-overview)
2. [Local development](#local-development)
3. [Project layout](#project-layout)
4. [Configuration & environment](#configuration--environment)
5. [Storage backend](#storage-backend)
6. [Database & migrations](#database--migrations)
7. [PDF → EPUB conversion stack](#pdf--epub-conversion-stack)
8. [App routes](#app-routes)
9. [API surface](#api-surface)
10. [Testing](#testing)
11. [Deployment plan](#deployment-plan)

---

## Architecture overview

Moco is intentionally boring — a single Go binary, a single SQLite file, and a pluggable storage backend.

| Layer            | Tech                                                                              |
|------------------|-----------------------------------------------------------------------------------|
| HTTP             | `net/http` (no framework)                                                         |
| Templating       | `html/template` (server-rendered) + small JS for progressive enhancement          |
| Database         | SQLite via `modernc.org/sqlite` (pure-Go driver, no CGO required to build)        |
| File storage     | Pluggable: local filesystem **or** Cloudflare R2 / any S3-compatible bucket       |
| Auth             | argon2id passwords, server-side sessions, CSRF (cookie + `X-CSRF-Token` header)   |
| Markdown parser  | `goldmark` (CommonMark + GFM + footnotes)                                         |
| Reader frontends | `pdf.js` (PDF), `epub.js` + `jszip` (EPUB), goldmark-rendered HTML (Markdown)     |
| Search           | SQLite `LIKE` for now — fast enough for personal libraries                        |

The plumbing fits on the smallest VM. CGO is only needed at the Docker layer because Calibre / mupdf are pulled in as binaries (not Go-linked).

### Request flow

```
client ── HTTP ──▶ Server.Handler() ──┬─▶ static FS (embed.FS)
                                      ├─▶ html/template render
                                      └─▶ JSON API handlers
                                              │
                                              ▼
                                     store.Store (SQLite)
                                              │
                                              ▼
                                  storage.Backend (Local / R2)
```

Files (book originals, converted EPUBs, covers) live in **storage.Backend**. Everything else (users, sessions, progress, highlights, tags, wishlist, shares) lives in **SQLite**.

---

## Local development

Requirements:

- Go 1.25+
- Optional but recommended: `calibre`, `mupdf-tools`, `poppler-utils` (for PDF→EPUB; otherwise a pure-Go fallback runs with poor fidelity)

```sh
# 1. Clone + cd
# 2. Copy env template
cp .env.example .env

# 3. Run
go run ./cmd/moco
```

App is at <http://localhost:8080>. Hot reload is not built in — rerun on change. For UI iteration without full restart, the templates are embedded but you can edit and `go run` again in <2s.

### Optional dev tooling

```sh
# macOS — pick at least one for decent PDF conversion
brew install --cask calibre   # ⭐ best fidelity
brew install mupdf            # text + images
brew install poppler          # text only
```

---

## Project layout

```
.
├── cmd/moco/                # main entry point + .env loader + storage backend selection
├── db/                      # SQLite seed dumps for local repro (optional)
├── internal/
│   ├── auth/                # argon2id password hashing
│   ├── epub/                # MarkdownToEPUB, PDFToEPUB, metadata extraction
│   ├── reader/              # markdown parser + reading-time estimates
│   ├── server/              # HTTP handlers, middleware, templates, view models
│   │   └── web/             # embed.FS source
│   │       ├── static/      # styles.css, app.js, og-default.svg
│   │       └── templates/   # *.html
│   ├── storage/             # Backend interface + Local + R2 + Prefixed wrapper
│   └── store/               # SQLite store + migrations
│       └── migrations/      # 000N_*.sql, run in order at startup
└── var/                     # local data (gitignored): SQLite DB + (when local backend) book files
```

---

## Configuration & environment

All config is via env vars. `.env` is loaded automatically at startup (gitignored). See `.env.example` for the documented template.

| Variable                       | Default | What it does                                                                          |
|--------------------------------|---------|---------------------------------------------------------------------------------------|
| `MOCO_ADDR`                    | `:8080` | Bind address                                                                          |
| `MOCO_DATA_DIR`                | `var`   | Where SQLite + (when local backend) book files live                                   |
| `MOCO_DB_PATH`                 | `${MOCO_DATA_DIR}/moco.sqlite` | Override DB path                                              |
| `MOCO_SECURE_COOKIES`          | `false` | Set behind HTTPS so cookies get the `Secure` flag                                     |
| `MOCO_PUBLIC_URL`              | (auto)  | Canonical https URL for OG / Twitter Card meta tags                                   |
| `MOCO_STORAGE`                 | (auto)  | `local` forces filesystem even when R2 vars are set                                   |
| `MOCO_R2_ACCOUNT_ID`           |         | Cloudflare account ID                                                                 |
| `MOCO_R2_ACCESS_KEY_ID`        |         | R2 API token access key                                                               |
| `MOCO_R2_SECRET_ACCESS_KEY`    |         | R2 API token secret                                                                   |
| `MOCO_R2_BUCKET`               |         | Bucket name (e.g. `moco`)                                                             |
| `MOCO_STORAGE_PREFIX`          | (empty) | Prepends to every storage key — use `dev` / `prod` to share one bucket safely         |

When all four `MOCO_R2_*` are set, R2 is selected automatically. To force local: `MOCO_STORAGE=local`.

---

## Storage backend

Defined in `internal/storage/`:

```go
type Backend interface {
    Put(ctx, key, body io.Reader, contentType string, size int64) error
    Get(ctx, key) (io.ReadCloser, error)
    Stat(ctx, key) (size int64, exists bool, err error)
    Delete(ctx, key) error
    DeletePrefix(ctx, prefix) error
    LocalPath(ctx, key) (path string, cleanup func(), err error)
}
```

Two implementations + one wrapper:

- **`Local`** — `BaseDir + key` filesystem path. `LocalPath` returns the real path with `cleanup=nil`.
- **`R2`** — AWS SDK v2 against `https://<account>.r2.cloudflarestorage.com` with `region=auto`. `LocalPath` downloads the object to a temp file and returns a cleanup func.
- **`Prefixed`** — wraps any backend; transparently prepends a fixed prefix to every key. Used for `MOCO_STORAGE_PREFIX` so a single bucket can host `dev/`, `prod/`, etc.

### Streaming optimisation

`server.serveBackendObject(...)` short-circuits to `http.ServeFile` when the backend resolves to `Local` (possibly through `Prefixed`). This gives **range-request support**, which `pdf.js` uses to fetch pages on demand instead of pulling the whole PDF.

R2 streams via `Get` + `io.Copy` — no range support yet. Acceptable for personal libraries (10-50MB books). Future work: implement `Range` header pass-through to R2.

### Migrating local → R2

```sh
# After filling MOCO_R2_* in .env
go run ./cmd/moco -migrate-storage
```

Implementation: `Server.MigrateLocalToBackend` walks every book in the DB, uploads each file to the configured backend, and rewrites DB paths to logical keys (e.g. `books/u_xyz/b_abc/original.pdf`). Idempotent — already-keyed rows are skipped via `filepath.IsAbs` check.

---

## Database & migrations

`internal/store/migrations/000N_*.sql` files are embedded and applied in order at startup. Add a new migration:

```sh
# Pick the next number, prefix-padded to 4 digits
touch internal/store/migrations/0007_<topic>.sql
# Write idempotent SQL: CREATE TABLE IF NOT EXISTS, ALTER TABLE...ADD COLUMN with care
```

Existing schema highlights:

- `users(id, email, password_hash, display_name, ...)`
- `sessions(token, user_id, expires_at)`
- `books(id, user_id, title, author, format, visibility, storage_path, derived_epub_path, cover_path, reading_minutes, ...)` — `storage_path` stores a logical key
- `reading_progress(user_id, book_id, locator, progress_percent)`
- `highlights(id, user_id, book_id, locator, selected_text, color, note)`
- `bookmarks`, `book_tags`, `book_shares`, `wishlist`

---

## PDF → EPUB conversion stack

Defined in `internal/epub/pdf.go`. Walks tools in order, picks the first available:

| Tool                       | Quality                                | Why                                              |
|----------------------------|----------------------------------------|--------------------------------------------------|
| `ebook-convert` (Calibre)  | ★★★★★  | Real EPUB w/ TOC, vector + raster images, chapter detection |
| `mutool` (mupdf)           | ★★★★    | Text + raster images. Lightweight (~10MB)        |
| `pdftotext` (poppler)      | ★★★      | Text only. Lightweight (~20MB)                   |
| `ledongthuc/pdf` (pure-Go) | ★★        | Last resort. No images, sometimes jumbled text   |

The tier name isn't surfaced to users — the upload UI just promises "EPUB you can resize and theme."

Calibre runs **headless** in containers via `QT_QPA_PLATFORM=offscreen` (set in the `Dockerfile`). The Go invocation also explicitly sets it on `cmd.Env`, belt-and-suspenders.

### Tradeoffs

- Vector graphics (technical diagrams) only survive Calibre. mutool drops them silently.
- Some PDFs use custom font glyph mappings where bullets are encoded as `z` / `f`. There is no clean fix — it's a property of the source PDF.
- We post-process mutool output: extracted images that aren't referenced inline get appended as a "Figures" appendix chapter so they're at least recoverable.

---

## App routes

| Path                 | Purpose                                |
|----------------------|----------------------------------------|
| `/`                  | Marketing landing page                 |
| `/discover`          | Public shelf (guest-readable)          |
| `/signup` · `/login` | Auth                                   |
| `/settings`          | Account settings                       |
| `/app`               | User library dashboard                 |
| `/quotes`            | Every highlight you've made            |
| `/stats`             | Reading stats                          |
| `/books/{id}/read`   | The reader (PDF / EPUB / Markdown)     |
| `/manifest.webmanifest` · `/sw.js` | PWA assets                |

Top-nav navigation between dashboard / quotes / stats / discover is **SPA-style** (fetch + DOMParser + swap `<main>`, `pushState`). The reader and auth pages do full reloads — they have heavy per-page setup (epub.js / pdf.js).

---

## API surface

```
GET    /api/v1/health
POST   /api/v1/auth/signup | login | logout
GET    /api/v1/auth/me
PUT    /api/v1/auth/me              update display name
PUT    /api/v1/auth/password
DELETE /api/v1/auth/me

GET    /api/v1/books                        your library
GET    /api/v1/books/public                 public shelf
POST   /api/v1/books/upload
POST   /api/v1/books/inspect                metadata-only preview
GET    /api/v1/books/{id}/content
GET    /api/v1/books/{id}/cover
GET    /api/v1/books/{id}/download
GET    /api/v1/books/{id}/converted.epub
DELETE /api/v1/books/{id}
PUT    /api/v1/books/{id}/visibility
GET    /api/v1/books/{id}/progress      | PUT
GET    /api/v1/books/{id}/highlights    | POST
GET    /api/v1/books/{id}/bookmarks     | POST
DELETE /api/v1/books/{id}/bookmarks/{bookmarkID}
POST   /api/v1/books/{id}/tags
DELETE /api/v1/books/{id}/tags/{tag}
GET    /api/v1/books/{id}/shares        | POST
DELETE /api/v1/books/{id}/shares/{userID}

GET    /api/v1/wishlist
POST   /api/v1/wishlist/{id} | DELETE

PUT    /api/v1/highlights/{id} | DELETE
```

All non-GET endpoints require the `X-CSRF-Token` header (the value is stored in a cookie set on first response).

---

## Testing

```sh
go test ./...
```

Covered: auth, upload, visibility toggling, reading progress, highlight CRUD, reader format dispatch, fragment endpoints. Storage backends and conversion paths are not unit-tested today — they require external binaries / network calls.

---

## Deployment plan

### One-time setup

1. **Cloudflare R2**
   - Create bucket (e.g. `moco`)
   - Create an API token in *R2 → Manage R2 API Tokens*: permission **Object Read & Write**, scope to the bucket
   - Record the **Access Key ID**, **Secret Access Key**, and your **Account ID**
2. **Domain** (optional)
   - Point a record at your VM
   - Front with a TLS proxy (Caddy / Cloudflare / Nginx)
3. **Server**
   - Any Linux VM with Docker installed. The image is ~1GB (Calibre is the bulk).
   - 1 vCPU / 2GB RAM is plenty for personal use; conversion peaks at ~500MB transient.

### Two-environment setup (recommended)

Use **one R2 bucket** with key prefixes — simpler ops than two buckets, and you can adjust IAM later if needed.

| | Dev (your laptop) | Prod (VM) |
|-|-|-|
| `MOCO_STORAGE_PREFIX` | `dev` | `prod` |
| `MOCO_PUBLIC_URL` | (unset) | `https://moco.example.com` |
| `MOCO_SECURE_COOKIES` | `false` | `true` |
| `MOCO_DATA_DIR` | `var` | `/app/var` |

Both share the same `MOCO_R2_*` credentials. Keys land at `moco/dev/books/...` and `moco/prod/books/...` respectively.

### Deploy

```sh
# 1. Pull / clone the repo on the VM
git pull

# 2. Set up .env (production values, including MOCO_STORAGE_PREFIX=prod)
cp .env.example .env && vim .env

# 3. Build + start
docker compose up -d --build

# 4. Tail logs to confirm storage backend
docker logs -f moco
# Expect: "storage backend: r2 (bucket=moco) [prefix=prod]"
```

### First-time data migration (only if you ran with local storage previously)

```sh
docker compose run --rm moco moco -migrate-storage
```

### Backups

The only **truly stateful** thing on the VM is `var/moco.sqlite` (when using R2 storage). Snapshot it with `cron`:

```sh
# /etc/cron.daily/moco-backup
sqlite3 /path/to/var/moco.sqlite ".backup '/backups/moco-$(date +%F).sqlite'"
# then: rclone copy /backups/ r2:moco-backups/
```

R2 itself doesn't need backups for our use case — uploads are idempotent and users still own the originals on their devices.

### Updates

```sh
git pull
docker compose up -d --build
```

The migration runner inside `Server.New` applies any new SQL migrations idempotently. No manual schema steps.

### Health check

```sh
curl https://moco.example.com/api/v1/health
# {"status":"ok","app":"moco"}
```

Hook into your uptime monitor of choice (Uptime Kuma, Better Stack, etc.).

### Cleanup worker

A goroutine started in `Server.New` sweeps `os.TempDir()` hourly and removes Moco's PDF-conversion artefacts older than 24h. No cron needed.

### Scaling considerations

Moco is designed for one user / household / small group. If usage grows:

- **Reader requests** are cheap — they're just streaming static files from R2.
- **PDF conversion** is the only CPU-heavy path. Calibre needs ~30s for a 200-page PDF. Consider a worker queue (e.g. NATS) if you ever cross a few uploads/min.
- **SQLite** holds up to surprising scale for read-heavy workloads (libraries, highlights). If write contention shows up, switch to Postgres — the `store.Store` interface is small enough to swap.
