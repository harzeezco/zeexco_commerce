# Stage 1: Build
FROM golang:1.26.2-alpine AS builder

WORKDIR /src

# Copy dependency files
COPY go.mod go.sum ./

# Download and verify dependencies
RUN go mod download && go mod verify

# Copy application source
COPY . .

# Compile the Go API and background worker
RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=readonly \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /out/zeexco-api \
    ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=readonly \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /out/zeexco-worker \
    ./cmd/worker

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /out/zeexco-api /zeexco-api
COPY --from=builder /out/zeexco-worker /zeexco-worker

ENTRYPOINT ["/zeexco-api"]
