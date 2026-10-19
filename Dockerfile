# syntax=docker/dockerfile:1

ARG GO_IMAGE=golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH

FROM build AS build-api
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/pagelense-api ./cmd/api

FROM build AS build-web
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/pagelense-web ./cmd/web

# Backend: API on 8081, metrics/pprof on 9091.
FROM ${RUNTIME_IMAGE} AS api
COPY --from=build-api /out/pagelense-api /pagelense-api
ENV PORT=8081 ADMIN_PORT=9091
EXPOSE 8081 9091
USER nonroot:nonroot
ENTRYPOINT ["/pagelense-api"]

# Frontend: UI on 8080, metrics/pprof on 9090. Set API_URL to reach the backend.
FROM ${RUNTIME_IMAGE} AS web
COPY --from=build-web /out/pagelense-web /pagelense-web
ENV PORT=8080 ADMIN_PORT=9090
EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/pagelense-web"]
