FROM --platform=$TARGETPLATFORM golang:1.24-bookworm AS builder

ARG CLEARLINK_VERSION=dev

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y \
        gcc \
        g++ \
        make \
        git \
        pkg-config \
        librtlsdr-dev \
        libusb-1.0-0-dev \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY . .

RUN go mod download

RUN mkdir -p /out

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X version.Version=${CLEARLINK_VERSION}" \
    -o /out/broadcast \
    ./cmd/broadcast

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X version.Version=${CLEARLINK_VERSION}" \
    -o /out/listen \
    ./cmd/listen

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X version.Version=${CLEARLINK_VERSION}" \
    -o /out/server \
    ./cmd/server

RUN chmod +x /out/broadcast /out/listen /out/server

FROM scratch

COPY --from=builder /out/ /