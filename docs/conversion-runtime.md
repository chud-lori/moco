# Conversion Runtime

This document covers the deployment side of the PDF/Markdown to EPUB conversion pipeline implemented in this branch.

## What the app does

For uploads:

- `md -> epub` uses the in-repo AST-based renderer.
- `pdf -> epub` uses a staged pipeline:
  - classify the PDF as `digital`, `scanned`, `scanned_with_ocr`, or `mixed`
  - apply OCR normalization when appropriate
  - try structured extractors in order
  - fall back to hybrid text/image EPUB assembly for weak scanned pages
  - fall back again to `ebook-convert`, `mutool`, `pdftotext`, then pure Go extraction

## Runtime dependencies

Core OS packages:

- `calibre`
- `ghostscript`
- `mupdf-tools`
- `poppler-utils`
- `python3`
- `python3-pip`
- `qpdf`
- `tesseract-ocr`
- `unpaper`

Core Python packages:

- `pymupdf4llm`
- `ocrmypdf`
- `docling`
- `marker-pdf`

Optional Python/CLI package:

- `nougat-ocr`

## Docker

The repo Docker image installs the core stack by default.

Default build:

```bash
docker build -t moco .
```

Math-heavy/Nougat-enabled build:

```bash
docker build --build-arg MOCO_INSTALL_NOUGAT=true -t moco .
```

## Health and capability reporting

The app exposes converter availability via:

- startup logs
- `GET /api/v1/health`

The health payload includes the detected conversion capabilities, for example:

```json
{
  "status": "ok",
  "app": "moco",
  "conversion": {
    "python": true,
    "ocrmypdf": true,
    "mutool": true,
    "pdftotext": true,
    "pdftoppm": true,
    "ebookConvert": true,
    "pymupdf4llm": true,
    "docling": true,
    "marker": true,
    "nougat": false,
    "structuredEngines": ["pymupdf4llm", "docling", "marker"],
    "pageRenderers": ["mutool", "pdftoppm"]
  }
}
```

## Fallback behavior

If some tools are missing, uploads still work:

- no `ocrmypdf`: scanned PDFs skip OCR normalization
- no structured extractor: PDF conversion falls back to `ebook-convert` or lower tiers
- no page renderer: scanned page-image fallback is skipped
- no optional tools at all: pure-Go text extraction remains available as last resort

## Recommended production check

After deploy:

1. call `/api/v1/health`
2. verify the expected engines are present
3. upload one sample markdown file
4. upload one born-digital PDF
5. upload one scanned PDF
6. if enabled, upload one math-heavy PDF

## Fixture guidance

For deployment verification, keep at least these sample files outside the repo or in private ops fixtures:

- born-digital text PDF
- scanned novel page PDF
- mixed-layout PDF with images
- math-heavy PDF

The in-repo tests cover the pipeline logic and API wiring, but real converter quality still depends on the host toolchain and the actual input documents.
