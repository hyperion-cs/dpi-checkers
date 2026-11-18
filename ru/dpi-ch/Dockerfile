FROM golang:1.27-alpine AS builder
WORKDIR /build
RUN apk add --no-cache git

COPY . .

ARG VERSION=dev
RUN \
    go build \
    -o dpich \
    -ldflags "-s -w \
      -X github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version.Value=${VERSION} \
      -X github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config.containerBuild=true" \
    -trimpath \
    .

FROM alpine:3.24 AS runner
ENV XDG_CONFIG_HOME=/etc XDG_DATA_HOME=/var/lib
COPY --from=builder /build/dpich /usr/bin/dpich
COPY config/template.yaml /etc/dpich/config.yaml
WORKDIR /work

ENTRYPOINT ["/usr/bin/dpich"]
