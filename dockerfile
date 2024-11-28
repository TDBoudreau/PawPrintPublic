# ================================
# Stage 1: Build the Go Binary
# ================================
FROM golang:1.23-alpine AS builder

# Install necessary packages
RUN apk update && apk add --no-cache git

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to leverage Docker cache
COPY go.mod go.sum ./

# Download Go module dependencies
RUN go mod download

# Copy the entire project source code
COPY . .

# Build the Go binary
# - CGO_ENABLED=0 ensures a statically linked binary
# - GOOS and GOARCH target Linux
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o pawprintpublic ./cmd/web

# ================================
# Stage 2: Create the Final Image
# ================================
FROM alpine:latest

# Install necessary packages (e.g., ca-certificates)
RUN apk update && apk add --no-cache ca-certificates

# Create a non-root user for better security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Set the working directory inside the container
WORKDIR /app

# Copy the Go binary from the builder stage
COPY --from=builder /app/pawprintpublic .

# Copy the SQLite database
COPY --from=builder /app/pawprint.db .

# Copy the /data/input directory with all its subdirectories and files
COPY --from=builder /app/data/input /app/data/input

# Copy the /static directory with all its subdirectories and files
COPY --from=builder /app/static /app/static

# Copy the /tmp directory with all its subdirectories and files
COPY --from=builder /app/tmp /app/tmp

# Handle the /templates directory:
# - Only copy top-level files (exclude subdirectories)
COPY --from=builder /app/templates/*.tmpl /app/templates/

# Ensure the binary has execute permissions
RUN chmod +x pawprintpublic

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose the port your application listens on
# EXPOSE 8080

# Set the entry point to run the Go binary
ENTRYPOINT ["./pawprintpublic"]
