#!/bin/sh

echo -n "o-o IPv4: " && dig -4 TXT +short o-o.myaddr.l.google.com @ns1.google.com
echo -n "o-o IPv6: " && dig -6 TXT +short o-o.myaddr.l.google.com @ns1.google.com
ip addr show
echo -n "Facebook IPv4: " && wget -O - facebook.com 2>/dev/null | head -1
echo -n "Facebook IPv6: " && wget -6 -O - facebook.com 2>/dev/null | head -1
echo -n "log.tailscale.io IPv4:" && wget -4 log.tailscale.io:443 2>&1 | grep Connecting

(
    ./app/tailscaled --tun=userspace-networking --socks5-server=localhost:1055 &
    sleep 3
    ./app/tailscale up --authkey=${TAILSCALE_AUTHKEY}
) &

all_proxy=socks5://localhost:1055/ ./app/server
