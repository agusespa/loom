# syntax=docker/dockerfile:1

# Pure Go (no cgo), cross-compiled for the target platform. The frontend is
# plain files embedded in the binary, so there is no Node stage.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS server
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/loom ./cmd/loom

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=server /out/loom /app/loom
ENV DB_PATH=/app/data/loom.db \
    PORT=8080
EXPOSE 8080
VOLUME /app/data
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/loom", "healthcheck"]
ENTRYPOINT ["/app/loom"]
