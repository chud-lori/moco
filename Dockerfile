FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/moco ./cmd/moco

FROM debian:bookworm-slim

# PDF→EPUB conversion stack, ordered by fidelity. The Go server walks this
# chain and uses the first available tool — Calibre is the gold standard
# (preserves vector graphics, real chapter detection, table of contents).
#
#   ebook-convert (Calibre)  → best: text + raster + vector graphics
#   mutool        (mupdf)    → text + raster images
#   pdftotext     (poppler)  → text only
#
# All three are installed so conversions just work out of the box.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
        ca-certificates \
        calibre \
        mupdf-tools \
        poppler-utils \
        libegl1 \
        libopengl0 \
        libxcb-cursor0 \
 && rm -rf /var/lib/apt/lists/*

# Calibre's ebook-convert links Qt and tries to open a display by default. In
# a container with no X server this would fail at runtime; offscreen forces
# Qt to render headlessly into memory. Required for ebook-convert to run.
ENV QT_QPA_PLATFORM=offscreen

RUN useradd --system --create-home --uid 10001 moco

WORKDIR /app

COPY --from=builder /out/moco /usr/local/bin/moco

RUN mkdir -p /app/var && chown -R moco:moco /app

USER moco

ENV MOCO_ADDR=:8080
ENV MOCO_DATA_DIR=/app/var

EXPOSE 8080

CMD ["moco"]
