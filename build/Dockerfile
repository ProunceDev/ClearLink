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
        ca-certificates \
        librtlsdr-dev \
        libusb-1.0-0-dev \
        libopus-dev \
        libopusfile-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN mkdir -p /out

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X clearlink/internal/version.Version=${CLEARLINK_VERSION}" \
    -o /out/broadcast \
    ./cmd/broadcast

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X clearlink/internal/version.Version=${CLEARLINK_VERSION}" \
    -o /out/listen \
    ./cmd/listen

RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build \
    -trimpath \
    -ldflags="-s -w -X clearlink/internal/version.Version=${CLEARLINK_VERSION}" \
    -o /out/server \
    ./cmd/server

RUN chmod +x /out/broadcast /out/listen /out/server

FROM scratch

COPY --from=builder /out/ /