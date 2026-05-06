# moco

**A calm, private reader for your books.** Upload your PDFs, EPUBs, and Markdown files, then read them on any device with the same Kindle-like experience — themes, font controls, highlights, and progress that follows you between phone, tablet, and laptop.

> Your library, your shelves, your highlights. Nothing tracked, nothing shared unless you choose to.

---

## What you get

📚 **Your private library** — Upload books once, read them anywhere. PDF, EPUB, and Markdown all land in the same calm reading interface.

🌗 **Reader you'll actually want to use** — Light, Paper, Sepia, and Dark themes. Adjust font size and line spacing. Reflowable text on any screen.

🔖 **Highlights and quotes** — Mark passages in any book. Search across every highlight you've ever made. Open the original spot in one tap.

📊 **Reading stats and progress** — See how much you've read, what you're partway through, and pick up exactly where you left off.

🤝 **Share what you want** — Keep books private, share a book with a specific friend by email, or publish to the public shelf for anyone to read.

✨ **One-click EPUB conversion** — Turn an awkward PDF or a plain Markdown file into a real reflowable EPUB on upload.

🏷️ **Tags + collections** — Organize books with custom tags. Filter your shelf by tag, format, or progress.

📥 **Want to read** — Save public books to your reading list. Build a wishlist of what's next.

---

## Quick start

The easiest way to run your own Moco:

```sh
docker compose up -d
```

Then open <http://localhost:8080>, sign up, and start uploading.

For local development without Docker:

```sh
go run ./cmd/moco
```

---

## Configuration

Copy the template and fill it in:

```sh
cp .env.example .env
```

Most things work out of the box. The two settings worth knowing:

- **Storage** — by default Moco stores book files on the local disk under `var/`. To use Cloudflare R2 (object storage, S3-compatible) instead, fill in the `MOCO_R2_*` variables in `.env` and Moco will switch automatically.
- **Dev / prod separation** — `MOCO_STORAGE_PREFIX=dev` (or `prod`, `staging`, etc.) prepends every key with that prefix, so a single bucket can host multiple environments cleanly.

For PDF→EPUB conversion to give you images and proper chapters, the Docker image already includes Calibre, mupdf-tools, and poppler-utils — no extra setup. For local Go development outside Docker, install one of:

```sh
# macOS — pick at least one (Calibre is the best)
brew install --cask calibre        # best fidelity
brew install mupdf                 # text + images
brew install poppler               # text only
```

---

## Migrating to R2

If you've been running Moco with local storage and want to move existing books to R2:

```sh
# 1. Fill in MOCO_R2_* in .env
# 2. Run the migration once:
go run ./cmd/moco -migrate-storage
```

Every book gets uploaded to your bucket, and the database is rewritten to point at object keys. The command is safe to re-run — already-migrated books are skipped.

---

## For developers

<details>
<summary>Tech stack &amp; architecture</summary>

- **Server**: Go 1.25, plain `net/http`, server-rendered `html/template` with a sprinkle of progressive-enhancement JS.
- **Database**: SQLite via `modernc.org/sqlite` (pure Go, no CGO needed for the build).
- **Storage**: pluggable backend interface — local filesystem or Cloudflare R2 / any S3-compatible store.
- **PDF/EPUB conversion**: chains Calibre → mupdf-tools → poppler-utils → a pure-Go fallback.
- **Reader frontends**: pdf.js, epub.js, goldmark for Markdown.
- **Auth**: argon2id passwords, server-side sessions, CSRF tokens via cookie + `X-CSRF-Token`.

The plumbing is intentionally boring — single binary, single SQLite file, fits on the smallest VM.

</details>

<details>
<summary>App routes</summary>

- `/` — landing
- `/discover` — public shelf
- `/signup` · `/login` · `/settings`
- `/app` — your library
- `/quotes` — every highlight you've made
- `/stats` — reading stats
- `/books/:id/read` — reader

</details>

<details>
<summary>API surface</summary>

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
GET    /api/v1/books/{id}/progress | PUT
GET    /api/v1/books/{id}/highlights | POST
GET    /api/v1/books/{id}/bookmarks | POST
POST   /api/v1/books/{id}/tags
DELETE /api/v1/books/{id}/tags/{tag}
GET    /api/v1/books/{id}/shares | POST
DELETE /api/v1/books/{id}/shares/{userID}

GET    /api/v1/wishlist | POST/DELETE /api/v1/wishlist/{id}
PUT    /api/v1/highlights/{id} | DELETE
```

</details>
