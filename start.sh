#!/bin/sh

hostname google-assistant-mqtt
export TZ='PST8PDT,M3.2.0/2:00:00,M11.1.0/2:00:00'

/app/tailscaled --tun=userspace-networking --socks5-server=localhost:1055 &
sleep 2
/app/tailscale up --authkey=${TAILSCALE_AUTHKEY} &
sleep 2

all_proxy=socks5://localhost:1055/ /app/smarthome
