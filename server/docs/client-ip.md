# Client IP forwarding

The API stores the client IP observed when a transfer is created and passes it
to the model input as `ip_address`.

## Direct/local mode

Leave `TRUSTED_PROXY_CIDRS` empty. The API uses `RemoteAddr`, which is normally
`127.0.0.1` or `::1` for local requests.

## Nginx + Cloudflare mode

The API must trust the address of the Nginx proxy as seen by the Go container,
not the Cloudflare CIDRs. Configure that address/network through:

```env
TRUSTED_PROXY_CIDRS=172.17.0.1/32
```

Replace the example with the actual source address visible from the API
container. Multiple CIDRs are comma-separated.

Nginx must only accept the real client IP from Cloudflare and overwrite the
forwarded header before proxying to Go. In the Nginx `http` block, configure
Cloudflare's current IP ranges with `set_real_ip_from` and use:

```nginx
real_ip_header CF-Connecting-IP;
real_ip_recursive on;

location / {
    proxy_set_header X-Real-IP $remote_addr;
    proxy_pass http://127.0.0.1:8080;
}
```

The Cloudflare ranges are intentionally not copied into the Go environment;
they belong in Nginx's `set_real_ip_from` configuration. Do not expose the Go
port directly to the Internet, because a client that can reach Go directly
could otherwise send trusted proxy headers.

The API trusts `CF-Connecting-IP` first and `X-Real-IP` as fallback only when
the TCP peer belongs to `TRUSTED_PROXY_CIDRS`. Requests from other peers ignore
both headers and use `RemoteAddr`.
