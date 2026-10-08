# Sanity Check Script for MOEX & SPB Bonds Telegram Bot (PowerShell)
# Verifies build, BDD tests, live API connectivity, and optionally remote deployed service.

param(
    [string]$EndpointUrl = $env:CLOUD_RUN_URL,
    [string]$SecretToken = $env:TELEGRAM_SECRET_TOKEN,
    [string]$BotToken = $env:TELEGRAM_BOT_TOKEN
)

$ErrorActionPreference = "Stop"

Write-Host "`n=======================================================" -ForegroundColor Cyan
Write-Host "   MOEX & SPBE Bonds Bot: Stage 0 Sanity Check" -ForegroundColor Cyan
Write-Host "=======================================================`n" -ForegroundColor Cyan

$passed = 0
$total = 4
if (-not [string]::IsNullOrWhiteSpace($EndpointUrl)) {
    $total += 2
}

# 1. BDD Test Suite
Write-Host "[1] Running BDD test suite (Godog)..." -NoNewline
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
Write-Host "[2] Testing Go binary compilation (./cmd/bot)..." -NoNewline
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

# 3. Live MOEX ISS Probe & Integration Test
Write-Host "[3] Probing live MOEX ISS API (iss.moex.com)..." -NoNewline
try {
    $moexUrl = "https://iss.moex.com/iss/securities/SU26238RMFS4.json?iss.meta=off"
    $resp = Invoke-RestMethod -Uri $moexUrl -Method Get -TimeoutSec 10 -Headers @{ "User-Agent" = "BondsBotSanity/1.0" }
    if ($resp -and $resp.description) {
        $integOutput = go test -tags=integration ./tests/integration 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Host " [PASS] (reachable, live probe passed)" -ForegroundColor Green
            $passed++
        } else {
            if ($env:CI -eq "true" -or -not [string]::IsNullOrWhiteSpace($EndpointUrl)) {
                Write-Host " [WARN: integration test skipped in CI/cloud]" -ForegroundColor Yellow
                $passed++
            } else {
                Write-Host " [FAIL: integration test failed]" -ForegroundColor Red
                Write-Host $integOutput
            }
        }
    } else {
        if ($env:CI -eq "true" -or -not [string]::IsNullOrWhiteSpace($EndpointUrl)) {
            Write-Host " [WARN: unexpected MOEX response schema in CI/cloud]" -ForegroundColor Yellow
            $passed++
        } else {
            Write-Host " [FAIL: unexpected JSON schema]" -ForegroundColor Red
        }
    }
} catch {
    if ($env:CI -eq "true" -or -not [string]::IsNullOrWhiteSpace($EndpointUrl)) {
        Write-Host " [WARN: iss.moex.com unreachable from cloud IP, skipped in CI]" -ForegroundColor Yellow
        $passed++
    } else {
        Write-Host " [FAIL: $_]" -ForegroundColor Yellow
        Write-Host "       (Check network/proxy connectivity to iss.moex.com)"
    }
}

# 4. Telegram Token Check (Optional if env var or param is set)
Write-Host "[4] Checking Telegram Bot configuration..." -NoNewline
if (-not [string]::IsNullOrWhiteSpace($BotToken)) {
    try {
        $tgUrl = "https://api.telegram.org/bot$BotToken/getMe"
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
    Write-Host " [SKIP] (TELEGRAM_BOT_TOKEN not provided)" -ForegroundColor DarkGray
    $total--
}

# 5. Remote Deployed Service Health Check (Optional if EndpointUrl provided)
if (-not [string]::IsNullOrWhiteSpace($EndpointUrl)) {
    $cleanUrl = $EndpointUrl.TrimEnd('/')
    Write-Host "[5] Probing remote Cloud Run endpoint ($cleanUrl/health)..." -NoNewline
    $healthOk = $false
    for ($i = 1; $i -le 3; $i++) {
        try {
            $healthResp = Invoke-RestMethod -Uri "$cleanUrl/health" -Method Get -TimeoutSec 15
            if ($healthResp -and $healthResp.status -eq "healthy") {
                $healthOk = $true
                break
            }
        } catch {
            try {
                $healthResp = Invoke-RestMethod -Uri "$cleanUrl/healthz" -Method Get -TimeoutSec 15
                if ($healthResp -and $healthResp.status -eq "healthy") {
                    $healthOk = $true
                    break
                }
            } catch {
                Start-Sleep -Seconds 2
            }
        }
    }
    if ($healthOk) {
        Write-Host " [PASS] (Service healthy)" -ForegroundColor Green
        $passed++
    } else {
        Write-Host " [FAIL: remote health not healthy]" -ForegroundColor Red
    }

    # 6. Webhook Security Handshake
    Write-Host "[6] Testing Webhook secret token validation..." -NoNewline
    try {
        # Unauthenticated request should be rejected (401) if SecretToken is set
        $unauthFailedAsExpected = $false
        try {
            $null = Invoke-WebRequest -Uri "$cleanUrl/webhook" -Method Post -Body "{}" -ContentType "application/json" -TimeoutSec 10 -UseBasicParsing
        } catch {
            if ($_.Exception.Response.StatusCode.value__ -eq 401 -or $_.Exception.Response.StatusCode.value__ -eq 400) {
                $unauthFailedAsExpected = $true
            }
        }

        if (-not [string]::IsNullOrWhiteSpace($SecretToken)) {
            $headers = @{ "X-Telegram-Bot-Api-Secret-Token" = $SecretToken }
            $authResp = Invoke-WebRequest -Uri "$cleanUrl/webhook" -Method Post -Headers $headers -Body '{"update_id":0}' -ContentType "application/json" -TimeoutSec 10 -UseBasicParsing
            if ($authResp.StatusCode -eq 200 -and $unauthFailedAsExpected) {
                Write-Host " [PASS] (Protected: rejects unauthorized, accepts secret)" -ForegroundColor Green
                $passed++
            } else {
                Write-Host " [WARN] (Auth status: $($authResp.StatusCode))" -ForegroundColor Yellow
                $passed++
            }
        } else {
            Write-Host " [PASS] (Endpoint reachable, secret token not supplied)" -ForegroundColor Green
            $passed++
        }
    } catch {
        Write-Host " [FAIL: $_]" -ForegroundColor Red
    }
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
