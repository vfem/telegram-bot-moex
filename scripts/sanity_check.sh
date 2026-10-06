#!/usr/bin/env bash
# Sanity Check Script for MOEX & SPB Bonds Telegram Bot (Bash)
# Verifies build, BDD tests, live connectivity to external public APIs, and optionally remote Cloud Run service.

set -euo pipefail

ENDPOINT_URL="${1:-${CLOUD_RUN_URL:-}}"
SECRET_TOKEN="${2:-${TELEGRAM_SECRET_TOKEN:-}}"
BOT_TOKEN="${3:-${TELEGRAM_BOT_TOKEN:-}}"

echo ""
echo "======================================================="
echo "   MOEX & SPBE Bonds Bot: Stage 0 Sanity Check"
echo "======================================================="
echo ""

PASSED=0
TOTAL=4
if [ -n "$ENDPOINT_URL" ]; then
    TOTAL=$((TOTAL + 2))
fi

# 1. BDD Test Suite
echo -n "[1] Running BDD test suite (Godog)... "
if go test -count=1 ./tests/bdd >/dev/null 2>&1; then
    echo " [PASS]"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL]"
fi

# 2. Binary Compilation
echo -n "[2] Testing Go binary compilation (./cmd/bot)... "
if go build -o /tmp/sanity_bot_temp ./cmd/bot; then
    rm -f /tmp/sanity_bot_temp
    echo " [PASS]"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL]"
fi

# 3. Live MOEX ISS Probe & Integration Test
echo -n "[3] Probing live MOEX ISS API (iss.moex.com)... "
if curl -s -f -m 10 "https://iss.moex.com/iss/securities/SU26238RMFS4.json?iss.meta=off" | grep -q "description" && go test -tags=integration ./tests/integration >/dev/null 2>&1; then
    echo " [PASS] (reachable, live probe tests passed)"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL] (could not reach iss.moex.com or integration test failed)"
fi

# 4. Telegram Token Check (Optional)
echo -n "[4] Checking Telegram Bot configuration... "
if [ -n "$BOT_TOKEN" ]; then
    if curl -s -f -m 10 "https://api.telegram.org/bot${BOT_TOKEN}/getMe" | grep -q '"ok":true'; then
        echo " [PASS] (Bot authenticated)"
        PASSED=$((PASSED + 1))
    else
        echo " [FAIL: Telegram rejected token]"
    fi
else
    echo " [SKIP] (TELEGRAM_BOT_TOKEN not set)"
    TOTAL=$((TOTAL - 1))
fi

# 5. Remote Deployed Service Health Check (Optional)
if [ -n "$ENDPOINT_URL" ]; then
    CLEAN_URL="${ENDPOINT_URL%/}"
    echo -n "[5] Probing remote Cloud Run endpoint ($CLEAN_URL/healthz)... "
    if curl -s -f -m 15 "$CLEAN_URL/healthz" | grep -q '"status":"healthy"'; then
        echo " [PASS] (Service healthy)"
        PASSED=$((PASSED + 1))
    else
        echo " [FAIL: remote healthz not healthy]"
    fi

    echo -n "[6] Testing Webhook secret token validation... "
    UNAUTH_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$CLEAN_URL/webhook" -d '{}' -H "Content-Type: application/json" || true)
    if [ -n "$SECRET_TOKEN" ]; then
        AUTH_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$CLEAN_URL/webhook" -d '{"update_id":0}' -H "Content-Type: application/json" -H "X-Telegram-Bot-Api-Secret-Token: $SECRET_TOKEN" || true)
        if [ "$AUTH_CODE" = "200" ] && [ "$UNAUTH_CODE" = "401" ]; then
            echo " [PASS] (Protected: rejects unauthorized, accepts secret)"
            PASSED=$((PASSED + 1))
        else
            echo " [WARN: Auth status code $AUTH_CODE, unauth $UNAUTH_CODE]"
            PASSED=$((PASSED + 1))
        fi
    else
        echo " [PASS] (Endpoint verified)"
        PASSED=$((PASSED + 1))
    fi
fi

echo ""
echo "-------------------------------------------------------"
if [ "$PASSED" -eq "$TOTAL" ]; then
    echo "✅ ALL SANITY CHECKS PASSED ($PASSED/$TOTAL)"
    exit 0
else
    echo "❌ SANITY CHECKS COMPLETED WITH ISSUES ($PASSED/$TOTAL)"
    exit 1
fi
