# INFRA

this is a folder with atleast my setup for having nextcloud on my local server running on my local ip, and through tailscale to remote domain.


/etc/hosts
```sh
# this is hre so that if one is on the local network, they can access nextcloud through the local ip.
{nextcloud_ip} domain.tld
```

Trust Caddy CA Certificate

```sh
# Pull and trust system-wide
ssh user@192.x.x.x "docker exec nextcloud-caddy cat /data/caddy/pki/authorities/local/root.crt" > ~/caddy-root.crt
sudo cp ~/caddy-root.crt /etc/pki/ca-trust/source/anchors/caddy-root.crt
sudo update-ca-trust

# Register in Chromium/Vivaldi NSS database
certutil -d sql:$HOME/.pki/nssdb -A -t "C,," -n "Caddy Local Root" -i ~/caddy-root.crt
```