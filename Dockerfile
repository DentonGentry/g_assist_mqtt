FROM golang:1.16-buster as builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . ./
RUN go build -mod=readonly -v -o server


FROM golang:1.16-buster as tailscale
WORKDIR /go/src/tailscale
RUN git clone https://github.com/tailscale/tailscale.git && \
    cd tailscale && go mod download && go install -mod=readonly ./cmd/tailscaled ./cmd/tailscale
COPY . ./


# Debian slim
# https://hub.docker.com/_/debian
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM debian:buster-slim
RUN set -x && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*


# Copy binary to production image
COPY --from=builder /app/server /app/server
COPY --from=builder /app/start.sh /app/start.sh
COPY --from=tailscale /go/bin/tailscaled /app/tailscaled
COPY --from=tailscale /go/bin/tailscale /app/tailscale
RUN mkdir -p /var/run/tailscale
RUN mkdir -p /var/cache/tailscale
RUN mkdir -p /var/lib/tailscale


# Run on container startup.
CMD ["/app/start.sh"]
