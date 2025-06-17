# Stage 1: build the manager binary
FROM golang:1.22-alpine AS builder
WORKDIR /workspace

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -o manager cmd/main.go

# Stage 2: create the final lightweight image
FROM alpine:3.18
RUN apk add --no-cache ca-certificates
WORKDIR /

# Copy the manager binary
COPY --from=builder /workspace/manager .

# Use a non-root user
USER 65532:65532

ENTRYPOINT ["/manager"]
