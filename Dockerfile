# Build stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine@sha256:b8ded51bad03238f67994d0a6b88680609b392db04312f60c23358cc878d4902 AS builder

# Define ARGs and ENVs to specify the target platform
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod to download dependencies
COPY go.mod ./
# Download dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Compile the application to a binary with all dependencies included
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o /main .

# Final stage
FROM scratch

# Copy the compiled binary from the builder stage
COPY --from=builder /main /main

# Set the binary as the entrypoint of the container
ENTRYPOINT ["/main"]

# Expose port 8080
EXPOSE 8080