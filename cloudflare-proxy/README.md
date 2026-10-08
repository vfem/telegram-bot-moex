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

## Security Model & Hardening

1. **Strict Route Gating:**
   - Only `POST /webhook` is forwarded upstream. All other paths (such as `/cron/daily-digest` or arbitrary routes) are rejected with `404 Not Found`.
   - GET `/health` and `/healthz` return `200 OK` without hitting Cloud Run.
2. **Constant-Time Secret Verification:**
   - Compares `X-Telegram-Bot-Api-Secret-Token` using `crypto.subtle.timingSafeEqual` to avoid timing side-channels.
3. **Error Masking & Upstream Shielding:**
   - Any STS or IAM exchange failure is logged to Cloudflare logs (`wrangler tail`) only.
   - HTTP clients receive a generic `502 Bad Gateway` without leaking Google STS / IAM JSON structures, project numbers, or SA emails.

## Secrets Management

Secrets are **never** committed to this repository. They are stored encrypted in Cloudflare's secrets store:

```bash
# Upload Telegram Secret Token
npx wrangler secret put TELEGRAM_SECRET_TOKEN

# Upload Private Signing Key
npx wrangler secret put PRIVATE_KEY_PEM
```

## Key Rotation Runbook

To rotate the RSA signing key used by Cloudflare Worker WIF:
1. Generate a new RSA keypair (outside this repository):
   ```bash
   openssl genrsa -out cf-private-new.pem 2048
   ```
2. Export the public JWK with `kid: "cf-key-2"`.
3. Update the GCP OIDC provider JWKS with both keys (`cf-key-1` and `cf-key-2`):
   ```bash
   gcloud iam workload-identity-pools providers update-oidc cf-worker-provider \
     --project=<PROJECT_ID> --location=global --workload-identity-pool=cloudflare-pool \
     --jwk-json-path=path/to/combined-jwks.json
   ```
4. Update the Worker private key secret:
   ```bash
   npx wrangler secret put PRIVATE_KEY_PEM < cf-private-new.pem
   ```
5. Update `kid` in `cloudflare-proxy/index.js` to `cf-key-2` and redeploy.
6. Verify end-to-end webhook delivery, then retire `cf-key-1` from the provider JWKS.

## Deployment

```bash
npx wrangler deploy
```
