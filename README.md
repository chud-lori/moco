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
cp .env.example .env
docker compose up -d
```

Then open <http://localhost:8080>, sign up, and start uploading.

---

## Configuration

All settings live in `.env` — copy `.env.example` and fill in what you need. The defaults work out of the box for local use.

The two settings worth knowing about:

- **Storage** — by default Moco stores book files on the local disk under `var/`. To use Cloudflare R2 (object storage) instead, fill in the `MOCO_R2_*` variables and Moco switches automatically.
- **Dev / prod separation** — `MOCO_STORAGE_PREFIX=dev` (or `prod`, `staging`) prepends every key with that prefix, so a single bucket can host multiple environments cleanly.

---

## For developers

Architecture, project layout, API surface, deployment plan, migration to R2, and contribution notes live in **[DEVELOPER.md](./DEVELOPER.md)**.
