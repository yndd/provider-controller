# Build the manager binary
FROM golang:1.17 as builder
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
# Copy the go source
COPY cmd/ cmd/
#COPY apis/ apis/
COPY internal/ internal/
COPY pkg/ pkg/
# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager cmd/controllercmd/main.go
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM alpine:latest
RUN apk add --update && \
    apk add --no-cache openssh && \
    apk add curl && \
    apk add tcpdump && \
    apk add iperf3 &&\
    apk add netcat-openbsd && \
    apk add ethtool && \
    apk add bonding && \
    rm -rf /tmp/*/var/cache/apk/*

RUN curl -sL https://github.com/karimra/gnmic/raw/master/install.sh | sh
RUN GRPC_HEALTH_PROBE_VERSION=v0.3.1 && \
    wget -qO/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 && \
    chmod +x /bin/grpc_health_probe
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
