#!/usr/bin/env bash
# Deployment Automation Script for GCP Cloud Run (Bash)
# Enforces GCP Free Tier parameters and completes full end-to-end setup.

set -euo pipefail

PROJECT="${1:-$(gcloud config get-value project 2>/dev/null || true)}"
REGION="${2:-europe-west1}"
SERVICE_NAME="${3:-moex-bonds-bot}"
BOT_TOKEN="${TELEGRAM_BOT_TOKEN:-}"
SECRET_TOKEN="${TELEGRAM_SECRET_TOKEN:-}"

echo ""
echo "======================================================="
echo "   MOEX & SPBE Bonds Bot: Cloud Run Deployment"
echo "   Target Cost: \$0.00 / month (GCP Free Tier)"
echo "======================================================="
echo ""

if [ -z "$PROJECT" ]; then
    echo "❌ Error: No GCP project selected. Run 'gcloud config set project <PROJECT_ID>'."
    exit 1
fi

echo "[1/6] Target GCP Project: $PROJECT in region $REGION"

if [ -z "$SECRET_TOKEN" ]; then
    SECRET_TOKEN=$(head -c 16 /dev/urandom | xxd -p)
    echo "      Generated random secret webhook token."
fi

if [ -z "$BOT_TOKEN" ]; then
    echo "      [WARNING] TELEGRAM_BOT_TOKEN not provided! Bot will run in mock mode."
fi

echo -n "[2/6] Enabling required GCP APIs... "
gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com cloudscheduler.googleapis.com --project "$PROJECT" >/dev/null 2>&1
echo " [OK]"

echo "[3/6] Deploying container to Cloud Run with Free Tier parameters..."
ENV_VARS="TELEGRAM_SECRET_TOKEN=$SECRET_TOKEN"
if [ -n "$BOT_TOKEN" ]; then
    ENV_VARS="$ENV_VARS,TELEGRAM_BOT_TOKEN=$BOT_TOKEN"
fi

gcloud run deploy "$SERVICE_NAME" \
    --source . \
    --project "$PROJECT" \
    --region "$REGION" \
    --platform managed \
    --no-allow-unauthenticated \
    --memory 128Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 2 \
    --concurrency 80 \
    --timeout 15s \
    --set-env-vars "$ENV_VARS"

SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" --project "$PROJECT" --region "$REGION" --format "value(status.url)")
echo ""
echo "[4/6] Service deployed successfully at: $SERVICE_URL"

if [ -n "$BOT_TOKEN" ]; then
    echo -n "[5/6] Configuring Telegram Webhook... "
    WEBHOOK_URL="${TELEGRAM_WEBHOOK_URL:-https://holy-art-8543.lkane516.workers.dev/webhook}"
    TG_RESP=$(curl -s -X POST "https://api.telegram.org/bot${BOT_TOKEN}/setWebhook" \
        -H "Content-Type: application/json" \
        -d "{\"url\":\"${WEBHOOK_URL}\",\"secret_token\":\"${SECRET_TOKEN}\",\"drop_pending_updates\":true}")
    if echo "$TG_RESP" | grep -q '"ok":true'; then
        echo " [PASS]"
    else
        echo " [FAIL: $TG_RESP]"
    fi
else
    echo "[5/6] Skipping Telegram Webhook registration (BOT_TOKEN empty)"
fi

echo -n "[6/6] Configuring Cloud Scheduler (06:00 UTC / 09:00 MSK)... "
SCHEDULER_JOB="moex-daily-digest"
if gcloud scheduler jobs describe "$SCHEDULER_JOB" --location "$REGION" --project "$PROJECT" >/dev/null 2>&1; then
    gcloud scheduler jobs update http "$SCHEDULER_JOB" \
        --location "$REGION" \
        --project "$PROJECT" \
        --schedule="0 6 * * *" \
        --uri="${SERVICE_URL}/cron/daily-digest" \
        --http-method=POST \
        --oidc-service-account-email="github-deployer@${PROJECT}.iam.gserviceaccount.com" \
        --oidc-token-audience="${SERVICE_URL}" >/dev/null 2>&1
    echo " [UPDATED]"
else
    gcloud scheduler jobs create http "$SCHEDULER_JOB" \
        --location "$REGION" \
        --project "$PROJECT" \
        --schedule="0 6 * * *" \
        --uri="${SERVICE_URL}/cron/daily-digest" \
        --http-method=POST \
        --oidc-service-account-email="github-deployer@${PROJECT}.iam.gserviceaccount.com" \
        --oidc-token-audience="${SERVICE_URL}" >/dev/null 2>&1
    echo " [CREATED]"
fi

echo ""
echo "Running post-deployment sanity check..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/sanity_check.sh" "$SERVICE_URL" "$SECRET_TOKEN" "$BOT_TOKEN"

echo ""
echo "======================================================="
echo "🎉 DEPLOYMENT COMPLETE!"
echo "   Service URL:    $SERVICE_URL"
echo "   Health Check:   $SERVICE_URL/health"
echo "   Webhook:        $SERVICE_URL/webhook"
echo "======================================================="
