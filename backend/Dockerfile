# Backend: one binary (HTTP API of the mini-app + bot). Build context is the repository root.
# The Go version is pinned: go.mod requires 1.27.1.

FROM golang:1.27-alpine AS build
WORKDIR /src

# Dependencies first: this layer is reused while go.mod/go.sum do not change.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

# Minimal runtime: no shell, no package manager, runs as a non-root user.
# Migrations are embedded in the binary and applied on start.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
