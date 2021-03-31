FROM golang:1.16.2-alpine3.13 as builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . ./
RUN go build -mod=readonly -v ./...


FROM golang:1.16.2-alpine3.13 as tailscale
WORKDIR /go/src/tailscale
COPY . ./
RUN apk update && apk add git
RUN git clone https://github.com/tailscale/tailscale.git && cd tailscale && go mod vendor && \
    rm wgengine/monitor/monitor_linux.go && \
    cat wgengine/monitor/monitor_polling.go | sed -e "s/+build .linux,/+build /" >wgengine/monitor/monitor_polling.go.new && \
    mv wgengine/monitor/monitor_polling.go.new wgengine/monitor/monitor_polling.go && \
    go install -mod=vendor ./cmd/tailscaled ./cmd/tailscale
COPY . ./


# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM alpine:latest
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

# Copy binary to production image
COPY --from=builder /app/smarthome /app/smarthome
COPY --from=builder /app/start.sh /app/start.sh
COPY --from=tailscale /go/bin/tailscaled /app/tailscaled
COPY --from=tailscale /go/bin/tailscale /app/tailscale
RUN mkdir -p /var/run/tailscale
RUN mkdir -p /var/cache/tailscale
RUN mkdir -p /var/lib/tailscale


# Run on container startup.
CMD ["/app/start.sh"]
