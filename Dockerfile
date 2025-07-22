# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine@sha256:daae04ebad0c21149979cd8e9db38f565ecefd8547cf4a591240dc1972cf1399 AS builder

# Define ARGs to specify the target platform and build metadata
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILD_DATE
ARG VCS_REF

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod to download dependencies
COPY go.mod ./
# Download dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Compile the application to a binary with all dependencies included
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -a -o /main cmd/echo-app/main.go

# Final stage
FROM scratch

# Re-declare ARGs in the final stage to use them
ARG BUILD_DATE
ARG VCS_REF

# Add metadata to the image using opencontainers labels
LABEL org.opencontainers.image.title="echo-app" \
      org.opencontainers.image.description="Tiny golang app which returns a timestamp, a customizable message, the hostname, the request source IP, and optionally the HTTP request headers." \
      org.opencontainers.image.url="https://github.com/philipschmid/echo-app" \
      org.opencontainers.image.source="https://github.com/philipschmid/echo-app" \
      org.opencontainers.image.vendor="philipschmid" \
      org.opencontainers.image.licenses="Apache-2.0 license" \
      org.opencontainers.image.revision="$VCS_REF" \
      org.opencontainers.image.created="$BUILD_DATE"

# Copy the compiled binary from the builder stage
COPY --from=builder /main /main

# Set the binary as the entrypoint of the container
ENTRYPOINT ["/main"]

# Expose port 8080
EXPOSE 8080