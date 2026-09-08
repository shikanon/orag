FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:233761595746769ebfdb6090f44fc7cdf818ae0ce62d2b37e0367723b9823e36 AS build
WORKDIR /src
COPY console/package.json console/package-lock.json ./
RUN npm ci
COPY console/ ./
RUN npm run build

FROM nginx:1.31.5-alpine@sha256:72ba65eb42c10344912a84ff42408db7d34f2feb642204570ab8fc5ffd29f1d3 AS console
RUN apk upgrade --no-cache
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.source="https://github.com/shikanon/orag" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY deployments/nginx-console.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=12 CMD wget -q -O /dev/null http://127.0.0.1:8080/console-healthz || exit 1
