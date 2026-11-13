# Stage 1: Build
FROM golang:1.26.2-alpine AS builder

WORKDIR /src

# Copy dependency files
COPY go.mod go.sum ./

# Download and verify dependencies
RUN go mod download && go mod verify

# Copy application source
COPY . .

# Compile the Go API
RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=readonly \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w" \
    -o /out/zeexco-api \
    ./cmd/api

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /out/zeexco-api /zeexco-api

ENTRYPOINT ["/zeexco-api"]