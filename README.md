# moco

`moco` is a fast personal reader for `PDF`, `EPUB`, and `Markdown`, built for a single VM with `Go`, `SQLite`, and local file storage.

## Current State

This repo currently contains:

- a responsive server-rendered web app served by `Go`
- SQLite-backed auth, sessions, library metadata, public/private visibility, reading progress, and highlights
- authenticated dashboard with separate private and public shelves
- public discovery shelf for books intentionally published by users
- Markdown reading UI with richer parsing, section TOC, progress resume, and persistent rendered highlight overlays
- PDF and EPUB in-app reader routes wired for browser rendering
- local file uploads with Markdown-to-EPUB sidecar conversion
- CSRF protection on every unsafe request and production-ready secure cookie support
- automated Go tests covering auth, upload, visibility, progress, highlight, and reader format flows
- the initial product and architecture plan in `plan.md`

## Run

```sh
go run ./cmd/moco
```

Then open `http://localhost:8080`.

Books and the SQLite database will be stored under:

```text
var/
```

For production HTTPS deployments, enable secure cookies:

```sh
MOCO_SECURE_COOKIES=true go run ./cmd/moco
```

## PDF → EPUB Conversion

When uploading a `.pdf` or `.md`, you can opt-in to convert it to EPUB on the
server for a much better reading experience (reflow, font controls, themes).

The Docker image **already includes the full conversion stack** — Calibre's
`ebook-convert` (best fidelity: vector graphics, raster images, TOC, chapter
detection), with `mupdf-tools` and `poppler-utils` as fallbacks. Calibre runs
headless via `QT_QPA_PLATFORM=offscreen`, so no X server is needed.

For local development outside Docker, install at least one of:

| Tool                       | Quality                                | Install (macOS)                |
|----------------------------|----------------------------------------|--------------------------------|
| **Calibre** (`ebook-convert`) | Best — vector + raster + TOC + chapters | `brew install --cask calibre` |
| **mupdf-tools** (`mutool`)    | Good — text + raster images             | `brew install mupdf`         |
| **poppler-utils** (`pdftotext`) | OK — text only                        | `brew install poppler`       |

The Go server walks the chain and uses the first tool found; users uploading a
PDF just check "Convert to EPUB" and the rest is silent.

## Docker Deploy

Build and run with persisted local data:

```sh
docker compose up --build -d
```

This mounts the host directory:

```text
./var
```

into the container at:

```text
/app/var
```

So these stay persisted across restarts:

- uploaded books
- generated EPUB sidecars
- `moco.sqlite`

## App Routes

- `/`
- `/discover`
- `/signup`
- `/login`
- `/app`
- `/books/:id`
- `/books/:id/read`

## API Endpoints

- `GET /api/v1/health`
- `POST /api/v1/auth/signup`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/books`
- `GET /api/v1/books/public`
- `POST /api/v1/books/upload`
- `PUT /api/v1/books/:id/visibility`
- `GET /api/v1/books/:id/content`
- `GET /api/v1/books/:id/progress`
- `PUT /api/v1/books/:id/progress`
- `GET /api/v1/books/:id/highlights`
- `POST /api/v1/books/:id/highlights`
- `GET /api/v1/books/{id}/download`
- `GET /api/v1/books/{id}/converted.epub`
- `DELETE /api/v1/books/{id}`
- `DELETE /api/v1/highlights/:id`

## Remaining Gaps

1. Vendor `pdf.js` and `epub.js` locally if you want zero runtime CDN dependency
2. Add richer PDF text selection overlays instead of page-note-only highlighting
3. Expand EPUB/PDF keyboard shortcuts and TOC navigation polish
