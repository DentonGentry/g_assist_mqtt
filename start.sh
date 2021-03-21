#!/bin/sh

hostname google-assistant-mqtt

(
    ./app/tailscaled --tun=userspace-networking --socks5-server=localhost:1055 &
    sleep 3
    ./app/tailscale up --authkey=${TAILSCALE_AUTHKEY}
) &

all_proxy=socks5://localhost:1055/ ./app/server
