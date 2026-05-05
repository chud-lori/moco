FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/moco ./cmd/moco

FROM debian:bookworm-slim

RUN useradd --system --create-home --uid 10001 moco

WORKDIR /app

COPY --from=builder /out/moco /usr/local/bin/moco

RUN mkdir -p /app/var && chown -R moco:moco /app

USER moco

ENV MOCO_ADDR=:8080
ENV MOCO_DATA_DIR=/app/var

EXPOSE 8080

CMD ["moco"]

