# moco

`moco` is a fast personal reader for `PDF`, `EPUB`, and `Markdown`, built for a single VM with `Go`, `SQLite`, and local file storage.

## Current State

This repo currently contains:

- a responsive server-rendered web app served by `Go`
- SQLite-backed auth, sessions, library metadata, public/private visibility, reading progress, and highlights
- authenticated dashboard with separate private and public shelves
- public discovery shelf for books intentionally published by users
- Markdown reading UI with table of contents, progress resume, and highlights
- PDF and EPUB in-app reader routes wired for browser rendering
- local file uploads with Markdown-to-EPUB sidecar conversion
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

## Next Build Steps

1. Add EPUB and PDF in-app readers instead of download/detail-only handling
2. Tighten cookie security and add CSRF protection for production
3. Add richer Markdown parsing and persistent rendered highlight overlays
4. Vendor reader assets locally if you want zero runtime CDN dependency
5. Add automated tests for auth, upload, visibility, progress, and highlight flows
