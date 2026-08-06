# Multi-binary image: this module builds four commands (api, ingest,
# extract, migrate) from one go.mod. Decision: ship all four in a single
# image at /app/{api,ingest,extract,migrate} rather than one image per
# binary. Cloud Run resources select which binary runs via a `command`
# override — the Service (cmd/api) uses the image's default CMD below,
# Jobs (cmd/ingest, cmd/extract, cmd/migrate) override it. Rejected
# alternative: separate image per binary — not worth the registry/CI
# overhead for four small static binaries sharing one module with no
# independent release cadence.

FROM golang:1.25 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -o /out/api ./cmd/api && \
    go build -o /out/ingest ./cmd/ingest && \
    go build -o /out/extract ./cmd/extract && \
    go build -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12
WORKDIR /app

COPY --from=builder /out/api /out/ingest /out/extract /out/migrate ./
COPY migrations/ ./migrations/

CMD ["/app/api"]
