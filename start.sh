#!/bin/sh

hostname google-assistant-mqtt
/app/tailscaled --tun=userspace-networking --socks5-server=0.0.0.0:1055 &
sleep 2
/app/tailscale up --authkey=${TAILSCALE_AUTHKEY} &
sleep 2
all_proxy=socks5://localhost:1055/ /app/server
