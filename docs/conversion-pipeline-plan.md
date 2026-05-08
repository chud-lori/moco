# Conversion Pipeline Plan

Date: 2026-05-08
Branch: `conversion-pipeline-plan`

## Goal

Improve reading experience for both:

- Markdown uploads converted to EPUB
- PDF uploads converted to EPUB, including scanned books

The target is not “always produce pure reflow text no matter what.” The target is:

- best readable result
- predictable quality
- explicit fallback behavior when extraction quality is weak

## Current Problems

### Markdown -> EPUB

Current conversion is line-based and lossy:

- weak structure preservation
- limited support for lists, tables, code, images, blockquotes
- heading/chapter logic is heuristic

### PDF -> EPUB

Current conversion is converter-chain fallback only:

- no PDF-type detection
- no scan normalization stage
- no confidence scoring
- no per-page fallback
- no distinction between born-digital, mixed, scanned-with-OCR, and fully scanned PDFs

## Target Architecture

Use a two-stage model:

1. Extract a normalized intermediate document
2. Build EPUB from that intermediate document

Do not convert PDF directly to EPUB in one opaque step.

## Document Types

Every upload should be classified as one of:

- `md`
- `pdf_digital`
- `pdf_scanned`
- `pdf_scanned_with_ocr`
- `pdf_mixed`
- `pdf_math_heavy`

Classification is used to choose pipeline behavior, not just for analytics.

## Intermediate Representation

Add a normalized internal format for conversion output.

Suggested structure:

- document metadata
  - title
  - author
  - language
  - source format
  - detector/classifier result
  - converter used
  - confidence
- ordered blocks
  - heading
  - paragraph
  - list
  - quote
  - code
  - table
  - image
  - page_break
- chapter map
- asset manifest
  - extracted images
  - cover
  - original page fallback images

This IR becomes the single EPUB input.

## Markdown Pipeline

### Proposed rewrite

Replace line-based Markdown conversion with AST-based conversion using `goldmark`.

### Steps

1. Parse Markdown to AST
2. Extract metadata from frontmatter
3. Convert AST to normalized blocks
4. Generate chapter structure from heading hierarchy
5. Render XHTML from normalized blocks
6. Package EPUB with:
   - nav
   - stylesheet
   - cover page
   - content OPF

### Must support

- headings
- paragraphs
- ordered/unordered lists
- blockquotes
- fenced code blocks
- inline code
- links
- images
- tables
- horizontal rules
- footnotes where possible

### Output expectation

Markdown conversion should become the high-confidence path with deterministic output.

## PDF Pipeline

## Phase 1: Detection

Before conversion, inspect the PDF.

Per-page signals:

- extracted text length
- word count
- image count
- largest image area ratio
- total image area ratio
- whether page appears to be one full-page raster
- extracted text quality
  - garbage characters
  - abnormal whitespace
  - low alphanumeric density

Per-document classification:

- mostly scanned if >80% pages are scanned-like
- mixed if 20-80%
- digital if <20%
- scanned_with_ocr if image-dominant pages still contain weak OCR text
- math_heavy if equations/symbol density is unusually high

### Implementation note

Store this classification in conversion metadata so the UI and pipeline can use it.

## Phase 2: Scan Normalization

For scanned, mixed, or OCR-bad PDFs:

1. Run OCR normalization first
2. Keep normalized PDF as an intermediate artifact

Recommended tool:

- OCRmyPDF

Recommended behaviors:

- `--mode skip` for mixed PDFs with existing good text
- `--mode redo` for scanned-with-bad-OCR
- `--mode force` only when necessary
- deskew
- clean / clean-final
- optional `--unpaper-args '--layout double'` for two-page scan layouts when detected

This stage is for scan cleanup and OCR normalization, not final EPUB generation.

## Phase 3: Structured Extraction

After normalization, select extractor by document class.

### Default extractor paths

- `pdf_digital` or `pdf_mixed`
  - PyMuPDF4LLM first
- `pdf_scanned`
  - Docling or Marker
- `pdf_scanned_with_ocr`
  - OCRmyPDF `redo`, then PyMuPDF4LLM or Marker
- `pdf_math_heavy`
  - Nougat-style path or dedicated math-aware extractor

### Why

- PyMuPDF4LLM is strong for born-digital and mixed layouts
- Marker is strong on messy PDFs and structured markdown output
- Docling is useful for full-page OCR/layout-aware extraction
- Nougat is specialized, best reserved for math/scientific material

## Phase 4: Ensemble Scoring

For hard PDFs, run two candidate extractors and choose the better result.

Suggested scoring signals:

- heading continuity
- paragraph length distribution
- duplicate header/footer rate
- garbage symbol rate
- repeated line fragments
- table preservation
- image placement coverage
- chapter coherence
- OCR confidence if available

This should be heuristic first, not ML-based.

## Phase 5: EPUB Builder

Always build the final EPUB from the normalized IR.

### EPUB builder requirements

- semantic XHTML chapters
- proper nav and spine
- extracted images included as assets
- page-break anchors from PDF page boundaries
- optional inline notes for uncertain OCR regions

## Scanned PDF Fallback Strategy

This is the critical piece for “best reading experience.”

When text extraction quality is weak on specific pages:

- do not force bad OCR into pure reflow text only
- embed the original page image at that location
- optionally include OCR text below or behind it

This creates a hybrid EPUB:

- reflow text where confidence is good
- page-image fallback where OCR is poor

For many scanned books, this will read better than a fully reflowed but badly recognized EPUB.

## Confidence Model

Each conversion should emit:

- `classification`
- `converter_used`
- `overall_confidence`
- `pages_with_image_fallback`
- `warnings`

Example warnings:

- “Scanned source with weak OCR on 18 pages”
- “Multi-column layout may have been simplified”
- “Table structure preserved partially”

## Product / UX Changes

Do not present PDF conversion as uniformly “better.”

Recommended messaging:

- Markdown: `Convert to EPUB`
- Digital PDF: `Convert to reflowable EPUB`
- Scanned PDF: `Create readable EPUB (OCR + image fallback)`
- Hard/low-confidence cases: show conversion notes after completion

Also expose:

- converter used
- confidence summary
- whether fallback page images were inserted

## Storage Plan

Per book, support multiple derived artifacts:

- original file
- normalized OCR PDF
- extracted markdown/json IR
- final converted EPUB
- extracted assets
- conversion manifest

Suggested manifest fields:

- source format
- source hash
- pipeline version
- detector result
- normalization tool/version
- extractor tool/version
- EPUB builder version
- confidence metrics

## Repo Changes

### New packages/modules

- `internal/convert`
  - pipeline orchestration
- `internal/convert/detect`
  - PDF inspection and classification
- `internal/convert/ir`
  - normalized intermediate document model
- `internal/convert/md`
  - Markdown AST -> IR
- `internal/convert/pdf`
  - OCR normalization and extractor orchestration
- `internal/convert/epub`
  - IR -> EPUB packaging

### Existing files to refactor

- `internal/epub/markdown.go`
  - replace with AST-based conversion or move under new conversion package
- `internal/epub/pdf.go`
  - replace converter-chain direct output with orchestrated pipeline
- `internal/server/server.go`
  - upload flow should trigger conversion pipeline and persist metadata
- `internal/store/store.go`
  - add conversion metadata columns or companion table

## Rollout Phases

### Phase A

- Add IR
- Rewrite Markdown -> EPUB around AST
- No PDF changes yet

### Phase B

- Add PDF detector
- Persist classification and conversion metadata
- Keep current PDF converter as fallback

### Phase C

- Add OCR normalization stage
- Add one structured PDF extractor path
- Build EPUB from IR

### Phase D

- Add second extractor for hard PDFs
- Add ensemble scoring
- Add hybrid page-image fallback

### Phase E

- Add conversion diagnostics to UI
- Improve retry/reprocess flows

## Dependency Strategy

### Keep in Go

- pipeline orchestration
- IR
- EPUB packaging
- metadata persistence
- UI integration

### Use helper subprocesses for specialized document processing

- OCRmyPDF
- Marker / Docling / PyMuPDF4LLM

Reason:

- the strongest document-understanding tools here are not Go-native
- forcing everything into pure Go will lower output quality
- subprocess boundaries keep the app architecture manageable

## Success Criteria

Markdown:

- chapter structure preserved
- code/list/table rendering preserved
- deterministic EPUB output

PDF:

- better chapter and paragraph reconstruction
- improved scanned-book readability
- fewer broken headers/footers
- fewer unusable low-confidence conversions
- explicit fallback when OCR quality is weak

## Recommended First Implementation

If only one path is implemented first:

1. rewrite Markdown -> EPUB properly
2. add PDF detector
3. add OCRmyPDF normalization
4. feed normalized PDF into one structured extractor
5. build final EPUB from IR

That gives the biggest quality gain without requiring the full ensemble on day one.
