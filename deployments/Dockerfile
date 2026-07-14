FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOFLAGS=-tags=stdjson,gjson
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOFLAGS="${GOFLAGS}" go build \
    -ldflags "-s -w -X github.com/shikanon/orag/pkg/buildinfo.Version=${VERSION} -X github.com/shikanon/orag/pkg/buildinfo.Commit=${COMMIT} -X github.com/shikanon/orag/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/orag-api ./cmd/orag-api && \
    CGO_ENABLED=0 GOFLAGS="${GOFLAGS}" go build \
    -ldflags "-s -w -X github.com/shikanon/orag/pkg/buildinfo.Version=${VERSION} -X github.com/shikanon/orag/pkg/buildinfo.Commit=${COMMIT} -X github.com/shikanon/orag/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/oragctl ./cmd/oragctl && \
    CGO_ENABLED=0 GOFLAGS="${GOFLAGS}" go build \
    -ldflags "-s -w -X github.com/shikanon/orag/pkg/buildinfo.Version=${VERSION} -X github.com/shikanon/orag/pkg/buildinfo.Commit=${COMMIT} -X github.com/shikanon/orag/pkg/buildinfo.BuildTime=${BUILD_TIME}" \
    -o /out/orag-demo ./cmd/orag-demo

FROM alpine:3.22 AS api
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.source="https://github.com/shikanon/orag" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache ca-certificates curl && \
    addgroup -S -g 10001 orag && \
    adduser -S -D -H -u 10001 -G orag orag && \
    mkdir -p /demo && chown 10001:10001 /demo
WORKDIR /app
COPY --from=build /out/orag-api /out/oragctl /out/orag-demo /usr/local/bin/
COPY api /app/api
COPY migrations /app/migrations
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["orag-api"]
