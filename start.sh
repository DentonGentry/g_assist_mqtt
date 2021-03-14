#!/bin/sh

./app/tailscaled --tun=userspace-networking --socks5-server=localhost:1055 &
sleep 5
./app/tailscale up --authkey=${TAILSCALE_AUTHKEY}

all_proxy=socks5://localhost:1055/ ./app/server
