#!/usr/bin/env bash
# Sanity Check Script for MOEX & SPB Bonds Telegram Bot (Bash)
# Verifies build, BDD tests, and live connectivity to external public APIs.

set -euo pipefail

echo ""
echo "======================================================="
echo "   MOEX & SPBE Bonds Bot: Stage 0 Sanity Check"
echo "======================================================="
echo ""

PASSED=0
TOTAL=4

# 1. BDD Test Suite
echo -n "[1/4] Running BDD test suite (Godog)... "
if go test -count=1 ./tests/bdd >/dev/null 2>&1; then
    echo " [PASS]"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL]"
fi

# 2. Binary Compilation
echo -n "[2/4] Testing Go binary compilation (./cmd/bot)... "
if go build -o /tmp/sanity_bot_temp ./cmd/bot; then
    rm -f /tmp/sanity_bot_temp
    echo " [PASS]"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL]"
fi

# 3. Live MOEX ISS Probe & Integration Test
echo -n "[3/4] Probing live MOEX ISS API (iss.moex.com)... "
if curl -s -f -m 10 "https://iss.moex.com/iss/securities/SU26238RMFS4.json?iss.meta=off" | grep -q "description" && go test -tags=integration ./tests/integration >/dev/null 2>&1; then
    echo " [PASS] (reachable, live probe tests passed)"
    PASSED=$((PASSED + 1))
else
    echo " [FAIL] (could not reach iss.moex.com or integration test failed)"
fi

# 4. Telegram Token Check (Optional)
echo -n "[4/4] Checking Telegram Bot configuration... "
if [ -n "${TELEGRAM_BOT_TOKEN:-}" ]; then
    if curl -s -f -m 10 "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe" | grep -q '"ok":true'; then
        echo " [PASS] (Bot authenticated)"
        PASSED=$((PASSED + 1))
    else
        echo " [FAIL: Telegram rejected token]"
    fi
else
    echo " [SKIP] (TELEGRAM_BOT_TOKEN not set)"
    TOTAL=$((TOTAL - 1))
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
