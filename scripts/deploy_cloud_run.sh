#!/usr/bin/env bash
# Deployment Automation Script for GCP Cloud Run (Bash)
# Enforces GCP Free Tier parameters and completes full end-to-end setup.

set -euo pipefail

PROJECT="${1:-$(gcloud config get-value project 2>/dev/null || true)}"
REGION="${2:-europe-west1}"
SERVICE_NAME="${3:-moex-bonds-bot}"
BOT_TOKEN="${TELEGRAM_BOT_TOKEN:-}"
SECRET_TOKEN="${TELEGRAM_SECRET_TOKEN:-}"
RUNTIME_SA="${RUNTIME_SA:-bot-runtime@${PROJECT}.iam.gserviceaccount.com}"
SCHEDULER_SA="${SCHEDULER_SA:-scheduler-invoker@${PROJECT}.iam.gserviceaccount.com}"

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
gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com cloudscheduler.googleapis.com secretmanager.googleapis.com --project "$PROJECT" >/dev/null 2>&1
echo " [OK]"

echo "[3/6] Syncing tokens with Google Secret Manager..."
if [ -n "$BOT_TOKEN" ]; then
    if ! gcloud secrets describe telegram-bot-token --project "$PROJECT" >/dev/null 2>&1; then
        printf '%s' "$BOT_TOKEN" | gcloud secrets create telegram-bot-token --data-file=- --replication-policy=automatic --project "$PROJECT" >/dev/null 2>&1
    else
        printf '%s' "$BOT_TOKEN" | gcloud secrets versions add telegram-bot-token --data-file=- --project "$PROJECT" >/dev/null 2>&1
    fi
fi
if [ -n "$SECRET_TOKEN" ]; then
    if ! gcloud secrets describe telegram-secret-token --project "$PROJECT" >/dev/null 2>&1; then
        printf '%s' "$SECRET_TOKEN" | gcloud secrets create telegram-secret-token --data-file=- --replication-policy=automatic --project "$PROJECT" >/dev/null 2>&1
    else
        printf '%s' "$SECRET_TOKEN" | gcloud secrets versions add telegram-secret-token --data-file=- --project "$PROJECT" >/dev/null 2>&1
    fi
fi

echo "[4/6] Deploying container to Cloud Run with Free Tier parameters..."
deploy_args=(run deploy "$SERVICE_NAME"
    --source .
    --project "$PROJECT"
    --region "$REGION"
    --platform managed
    --no-allow-unauthenticated
    --memory 128Mi
    --cpu 1
    --min-instances 0
    --max-instances 2
    --concurrency 80
    --timeout 15s)

# Attach dedicated runtime SA if created
if gcloud iam service-accounts describe "$RUNTIME_SA" --project "$PROJECT" >/dev/null 2>&1; then
    deploy_args+=(--service-account "$RUNTIME_SA")
fi

# Attach secrets from Secret Manager if configured
if gcloud secrets describe telegram-bot-token --project "$PROJECT" >/dev/null 2>&1 && \
   gcloud secrets describe telegram-secret-token --project "$PROJECT" >/dev/null 2>&1; then
    deploy_args+=(--set-secrets "TELEGRAM_BOT_TOKEN=telegram-bot-token:latest,TELEGRAM_SECRET_TOKEN=telegram-secret-token:latest")
fi

gcloud "${deploy_args[@]}"

SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" --project "$PROJECT" --region "$REGION" --format "value(status.url)")
echo ""
echo "Service deployed successfully at: $SERVICE_URL"

if [ -n "$BOT_TOKEN" ]; then
    echo -n "[5/6] Configuring Telegram Webhook... "
    WEBHOOK_URL="${TELEGRAM_WEBHOOK_URL:-https://holy-art-8543.lkane516.workers.dev/webhook}"
    TG_RESP=$(curl -s -X POST "https://api.telegram.org/bot${BOT_TOKEN}/setWebhook" \
        -H "Content-Type: application/json" \
        -d "{\"url\":\"${WEBHOOK_URL}\",\"secret_token\":\"${SECRET_TOKEN}\",\"drop_pending_updates\":true,\"allowed_updates\":[\"message\"]}")
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
target_scheduler_sa="$SCHEDULER_SA"
if ! gcloud iam service-accounts describe "$target_scheduler_sa" --project "$PROJECT" >/dev/null 2>&1; then
    target_scheduler_sa="github-deployer@${PROJECT}.iam.gserviceaccount.com"
fi

if gcloud scheduler jobs describe "$SCHEDULER_JOB" --location "$REGION" --project "$PROJECT" >/dev/null 2>&1; then
    gcloud scheduler jobs update http "$SCHEDULER_JOB" \
        --location "$REGION" \
        --project "$PROJECT" \
        --schedule="0 6 * * *" \
        --uri="${SERVICE_URL}/cron/daily-digest" \
        --http-method=POST \
        --oidc-service-account-email="$target_scheduler_sa" \
        --oidc-token-audience="${SERVICE_URL}" >/dev/null 2>&1
    echo " [UPDATED]"
else
    gcloud scheduler jobs create http "$SCHEDULER_JOB" \
        --location "$REGION" \
        --project "$PROJECT" \
        --schedule="0 6 * * *" \
        --uri="${SERVICE_URL}/cron/daily-digest" \
        --http-method=POST \
        --oidc-service-account-email="$target_scheduler_sa" \
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
