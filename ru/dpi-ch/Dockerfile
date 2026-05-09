FROM golang:1.26-alpine AS builder
WORKDIR /build
RUN apk add --no-cache git

COPY . .

ARG VERSION=dev
RUN \
    go build \
    -o dpich \
    -ldflags "-s -w -X github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version.Value=${VERSION}" \
    -trimpath \
    .


FROM alpine:3.23 AS runner
COPY --from=builder /build/dpich /usr/bin/dpich
COPY docker/config.yaml /etc/dpich/config.yaml

ENTRYPOINT ["/usr/bin/dpich"]

# Download inetlookup data by default
CMD [ \
    "--cfg", "/etc/dpich/config.yaml", \
    "--force-inetlookup-update" \
    ]
