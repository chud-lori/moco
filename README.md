# moco

**A calm, private reader for your books.** Upload your PDFs, EPUBs, and Markdown files, then read them on any device with a polished reading experience — themes, font controls, highlights, bookmarks, and progress that follows you between phone, tablet, and laptop.

> Your library, your shelves, your highlights. Nothing tracked, nothing shared unless you choose to.

---

## What you get

📚 **Your private library** — Upload books once, read them anywhere. PDF, EPUB, and Markdown all land in the same calm reading interface.

🌗 **Reader you'll actually want to use** — Light, Paper, Sepia, and Dark themes. Adjust font size and line spacing. Reflowable text on any screen.

🔖 **Highlights and quotes** — Mark passages in any book. Search across every highlight you've ever made. Open the original spot in one tap.

📊 **Reading stats and progress** — See how much you've read, what you're partway through, and pick up exactly where you left off.

🤝 **Share what you want** — Keep books private, share a book with a specific friend by email, or publish to the public shelf for anyone to read.

✨ **One-click EPUB conversion** — Turn an awkward PDF or a plain Markdown file into a real reflowable EPUB on upload.

🧠 **Hybrid PDF conversion pipeline** — Born-digital PDFs prefer structured extraction, scanned PDFs can run through OCR, and hard pages can fall back to page images inside the EPUB instead of failing outright.

🏷️ **Tags + collections** — Organize books with custom tags. Filter your shelf by tag, format, or progress.

📥 **Want to read** — Save public books to your reading list. Build a wishlist of what's next.

---

## Try it

A live instance is running at **<https://moco.lori.my.id>**. Sign up with any email, upload a book, and you'll see the whole experience — themes, highlights, EPUB conversion, the lot.

---

## For developers

Architecture, project layout, API surface, configuration reference, deployment plan, and the R2 migration command live in **[DEVELOPER.md](./DEVELOPER.md)**.

## Conversion Deployment

The conversion pipeline in this branch expects a runtime stack, not just Go code. The included [Dockerfile](./Dockerfile) installs the default deployment toolchain:

- `ebook-convert`
- `ocrmypdf`
- `mutool`
- `pdftotext`
- `pdftoppm`
- `python3`
- `pymupdf4llm`
- `docling`
- `marker`

Optional:

- `nougat` for math-heavy PDFs. Enable it at build time with `--build-arg MOCO_INSTALL_NOUGAT=true`.

At runtime, check `GET /api/v1/health` or the startup logs to see which conversion engines were actually detected on the host.

More detail lives in [docs/conversion-runtime.md](./docs/conversion-runtime.md).
