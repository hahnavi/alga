# Production Deployment with Caddy HTTPS

## Quick Start

1. Copy `.env.example` to `.env` and configure values (run `./setup.sh` if available).
2. Set `DOMAIN=your-domain.com` in `.env`.
3. Start the stack:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Caddy automatically provisions TLS certificates via Let's Encrypt for the configured domain.

## Local Testing

For local testing without TLS, keep `DOMAIN=localhost` in `.env`. Caddy will serve HTTP only.

## Notes

- The production overlay hides direct backend (8080) and frontend (3000) port exposure. All traffic goes through Caddy on ports 80/443.
- Caddy data (certificates, etc.) is persisted in the `caddy_data` and `caddy_config` named volumes.
- All services are configured with `restart: always` and JSON file logging with 10 MB / 3 file rotation.
