# Stage 1: Build
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" \
    -o /bin/xp-tracker \
    ./cmd/exporter

# Stage 2: Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /bin/xp-tracker /xp-tracker

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/xp-tracker"]
