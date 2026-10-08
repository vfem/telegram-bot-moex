# Cloudflare Edge Proxy with Google Workload Identity Federation (WIF)

This Cloudflare Worker acts as a zero-cost, edge-level security shield for the MOEX & SPB Bonds Telegram Bot on Google Cloud Run.

## Architecture

1. **Telegram Webhook** ➔ Hits `https://<worker-name>.<subdomain>.workers.dev/webhook`.
2. **Edge Token Verification** ➔ Cloudflare checks `X-Telegram-Bot-Api-Secret-Token`.
   - If invalid / missing: Terminated immediately at edge with `401 Unauthorized` ($0.00, zero traffic to GCP).
3. **Google IAM Authentication** ➔ For valid updates, Cloudflare uses Workload Identity Federation to exchange a signed JWT with Google STS (`sts.googleapis.com`) for a Google IAM ID Token.
4. **Proxy to Private Cloud Run** ➔ Forwards request to Cloud Run with `Authorization: Bearer <ID_TOKEN>`.
   - Google Front End (GFE) verifies the Google IAM signature.
   - Any direct unauthenticated internet request to Cloud Run is rejected with `403 Forbidden` before container execution.

## Secrets Management

Secrets are **never** committed to this repository. They are stored encrypted in Cloudflare's secrets store:

```bash
# Upload Telegram Secret Token
npx wrangler secret put TELEGRAM_SECRET_TOKEN

# Upload Private Signing Key
npx wrangler secret put PRIVATE_KEY_PEM
```

## Deployment

```bash
npx wrangler deploy
```
