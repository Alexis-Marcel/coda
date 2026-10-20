# syntax=docker/dockerfile:1
# Image de Coda : un seul binaire Go qui sert aussi le front compilé.
# Le front et le binaire sont construits sur la machine de build (compilation croisée, pas d'émulation).

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS server
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/coda ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/coda /coda
COPY --from=web /web/dist /web
ENV ADDR=:8080 DATA_DIR=/data STATIC_DIR=/web
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/coda", "health"]
USER nonroot:nonroot
ENTRYPOINT ["/coda"]
