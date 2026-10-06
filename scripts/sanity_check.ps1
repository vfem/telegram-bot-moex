# Sanity Check Script for MOEX & SPB Bonds Telegram Bot (PowerShell)
# Verifies build, BDD tests, and live connectivity to external public APIs.

$ErrorActionPreference = "Stop"

Write-Host "`n=======================================================" -ForegroundColor Cyan
Write-Host "   MOEX & SPBE Bonds Bot: Stage 0 Sanity Check" -ForegroundColor Cyan
Write-Host "=======================================================`n" -ForegroundColor Cyan

$passed = 0
$total = 4

# 1. BDD Test Suite
Write-Host "[1/4] Running BDD test suite (Godog)..." -NoNewline
try {
    $testOutput = go test -count=1 ./tests/bdd 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host " [PASS]" -ForegroundColor Green
        $passed++
    } else {
        Write-Host " [FAIL]" -ForegroundColor Red
        Write-Host $testOutput
    }
} catch {
    Write-Host " [FAIL: $_]" -ForegroundColor Red
}

# 2. Binary Compilation
Write-Host "[2/4] Testing Go binary compilation (./cmd/bot)..." -NoNewline
try {
    $tempExe = Join-Path $PSScriptRoot "..\sanity_bot_temp.exe"
    $buildOutput = go build -o $tempExe ./cmd/bot 2>&1
    if ($LASTEXITCODE -eq 0 -and (Test-Path $tempExe)) {
        Remove-Item -Force $tempExe -ErrorAction SilentlyContinue
        Write-Host " [PASS]" -ForegroundColor Green
        $passed++
    } else {
        Write-Host " [FAIL]" -ForegroundColor Red
        Write-Host $buildOutput
    }
} catch {
    Write-Host " [FAIL: $_]" -ForegroundColor Red
}

# 3. Live MOEX ISS Probe
Write-Host "[3/4] Probing live MOEX ISS API (iss.moex.com)..." -NoNewline
try {
    $moexUrl = "https://iss.moex.com/iss/securities/SU26238RMFS4.json?iss.meta=off"
    $resp = Invoke-RestMethod -Uri $moexUrl -Method Get -TimeoutSec 10 -Headers @{ "User-Agent" = "BondsBotSanity/1.0" }
    if ($resp -and $resp.description) {
        Write-Host " [PASS] (reachable, response parsed)" -ForegroundColor Green
        $passed++
    } else {
        Write-Host " [FAIL: unexpected JSON schema]" -ForegroundColor Red
    }
} catch {
    Write-Host " [FAIL: $_]" -ForegroundColor Yellow
    Write-Host "       (Check network/proxy connectivity to iss.moex.com)"
}

# 4. Telegram Token Check (Optional if env var is set)
Write-Host "[4/4] Checking Telegram Bot configuration..." -NoNewline
$botToken = $env:TELEGRAM_BOT_TOKEN
if (-not [string]::IsNullOrWhiteSpace($botToken)) {
    try {
        $tgUrl = "https://api.telegram.org/bot$botToken/getMe"
        $tgResp = Invoke-RestMethod -Uri $tgUrl -Method Get -TimeoutSec 10
        if ($tgResp.ok) {
            Write-Host " [PASS] (Bot username: @$($tgResp.result.username))" -ForegroundColor Green
            $passed++
        } else {
            Write-Host " [FAIL: Telegram rejected token]" -ForegroundColor Red
        }
    } catch {
        Write-Host " [WARN: Telegram API unreachable or invalid token: $_]" -ForegroundColor Yellow
    }
} else {
    Write-Host " [SKIP] (TELEGRAM_BOT_TOKEN not set in environment)" -ForegroundColor DarkGray
    $total--
}

Write-Host "`n-------------------------------------------------------"
if ($passed -eq $total) {
    Write-Host "✅ ALL SANITY CHECKS PASSED ($passed/$total)" -ForegroundColor Green
    Write-Host "The bot code and infrastructure dependencies are healthy.`n"
    exit 0
} else {
    Write-Host "❌ SANITY CHECKS COMPLETED WITH WARNINGS ($passed/$total passed)" -ForegroundColor Yellow
    exit 1
}
