FROM golang:1.25-alpine AS builder
WORKDIR /build
RUN apk add --no-cache git

COPY . .

ARG VERSION=dev
RUN \
    go build \
    -o dpich \
    -ldflags "-X dpich/internal/version.Value=${VERSION}" \
    -trimpath \
    .


FROM alpine:3.23 AS runner
COPY --from=builder /build/dpich /usr/bin/dpich

ENTRYPOINT ["/usr/bin/dpich"]

# Download inetlookup data by default
CMD ["--force-inetlookup-update"]
