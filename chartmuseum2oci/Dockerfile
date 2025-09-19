FROM golang:1.20 AS builder

WORKDIR /root/src

ENV CGO_ENABLED=0 \
    GO111MODULE=on \
    GOOS="linux"

COPY go.mod go.sum ./
COPY main.go ./

RUN go build -a \
    -o /go/bin/chartmuseum2oci \
    main.go

############################

FROM alpine AS helm

ARG HELM_VERSION="v3.19.0"
ARG TARGETARCH

# Set default architecture if not provided
RUN if [ -z "$TARGETARCH" ]; then \
        ARCH=$(uname -m); \
        case $ARCH in \
            x86_64) TARGETARCH=amd64 ;; \
            aarch64|arm64) TARGETARCH=arm64 ;; \
            armv7l) TARGETARCH=arm ;; \
            *) echo "Unsupported architecture: $ARCH" && exit 1 ;; \
        esac; \
    fi
    wget -q https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz -O - \
    | tar -xzO linux-${TARGETARCH}/helm > /bin/helm && \
    chmod +x /bin/helm

############################

FROM gcr.io/distroless/static

WORKDIR /home/nonroot
USER nonroot:nonroot

COPY --from=builder /go/bin/chartmuseum2oci /chartmuseum2oci
COPY --from=helm /bin/helm /usr/local/bin/helm

ENTRYPOINT ["/chartmuseum2oci"]
