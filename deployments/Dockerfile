FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOFLAGS=-tags=stdjson,gjson
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOFLAGS="${GOFLAGS}" go build -o /out/orag-api ./cmd/orag-api

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/orag-api /usr/local/bin/orag-api
COPY api /app/api
EXPOSE 8080
ENTRYPOINT ["orag-api"]
