# Deployment Automation Script for GCP Cloud Run (PowerShell)
# Enforces GCP Free Tier parameters and completes full end-to-end setup.

param(
    [string]$Project = "",
    [string]$Region = "europe-west1",
    [string]$ServiceName = "moex-bonds-bot",
    [string]$BotToken = $env:TELEGRAM_BOT_TOKEN,
    [string]$SecretToken = $env:TELEGRAM_SECRET_TOKEN,
    [switch]$SkipWebhook = $false,
    [switch]$SkipScheduler = $false
)

$ErrorActionPreference = "Stop"

Write-Host "`n=======================================================" -ForegroundColor Cyan
Write-Host "   MOEX & SPBE Bonds Bot: Cloud Run Deployment" -ForegroundColor Cyan
Write-Host "   Target Cost: $0.00 / month (GCP Free Tier)" -ForegroundColor Cyan
Write-Host "=======================================================`n" -ForegroundColor Cyan

# 1. Check gcloud CLI
Write-Host "[1/6] Checking Google Cloud SDK..." -NoNewline
if (-not (Get-Command gcloud -ErrorAction SilentlyContinue)) {
    Write-Host " [FAIL]" -ForegroundColor Red
    Write-Error "gcloud CLI not found. Please install Google Cloud SDK."
}
Write-Host " [OK]" -ForegroundColor Green

# 2. Project & Auth resolution
if ([string]::IsNullOrWhiteSpace($Project)) {
    $Project = (gcloud config get-value project 2>$null).Trim()
}
if ([string]::IsNullOrWhiteSpace($Project)) {
    Write-Error "No GCP project selected. Specify -Project or run 'gcloud config set project <PROJECT_ID>'."
}
Write-Host "[2/6] Active GCP Project: $Project in region $Region" -ForegroundColor Yellow

# Secret token generation if missing
if ([string]::IsNullOrWhiteSpace($SecretToken)) {
    $rngBytes = New-Object byte[] 16
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($rngBytes)
    $SecretToken = ($rngBytes | ForEach-Object { "{0:x2}" -f $_ }) -join ""
    Write-Host "      Generated random secret webhook token." -ForegroundColor DarkGray
}

if ([string]::IsNullOrWhiteSpace($BotToken)) {
    Write-Host "      [WARNING] TELEGRAM_BOT_TOKEN not provided! Bot will run in mock mode." -ForegroundColor Yellow
}

# 3. Enable required GCP APIs (idempotent)
Write-Host "[3/6] Ensuring required GCP APIs are enabled..." -NoNewline
gcloud services enable run.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com cloudscheduler.googleapis.com --project $Project 2>&1 | Out-Null
Write-Host " [OK]" -ForegroundColor Green

# 4. Deploy to Cloud Run with Free Tier Guardrails
Write-Host "[4/6] Deploying container to Cloud Run with Free Tier parameters..." -ForegroundColor Cyan
Write-Host "      - Memory: 128Mi (minimum billing tier)" -ForegroundColor DarkGray
Write-Host "      - CPU: 1 vCPU with request-based throttling" -ForegroundColor DarkGray
Write-Host "      - Instances: min 0 (scale-to-zero), max 2" -ForegroundColor DarkGray
Write-Host "      - Concurrency: 80, Timeout: 15s" -ForegroundColor DarkGray

$envVars = "TELEGRAM_SECRET_TOKEN=$SecretToken"
if (-not [string]::IsNullOrWhiteSpace($BotToken)) {
    $envVars += ",TELEGRAM_BOT_TOKEN=$BotToken"
}

gcloud run deploy $ServiceName `
    --source . `
    --project $Project `
    --region $Region `
    --platform managed `
    --allow-unauthenticated `
    --memory 128Mi `
    --cpu 1 `
    --min-instances 0 `
    --max-instances 2 `
    --concurrency 80 `
    --timeout 15s `
    --set-env-vars $envVars

if ($LASTEXITCODE -ne 0) {
    Write-Error "Cloud Run deployment failed."
}

# 5. Extract Service URL
$serviceUrl = (gcloud run services describe $ServiceName --project $Project --region $Region --format "value(status.url)").Trim()
Write-Host "`n[5/6] Service deployed successfully at: $serviceUrl" -ForegroundColor Green

# 6. Webhook and Scheduler setup
if (-not $SkipWebhook -and -not [string]::IsNullOrWhiteSpace($BotToken)) {
    Write-Host "[6a/6] Configuring Telegram Webhook..." -NoNewline
    $webhookEndpoint = "$serviceUrl/webhook"
    try {
        $setWebhookUrl = "https://api.telegram.org/bot$BotToken/setWebhook"
        $body = @{
            url = $webhookEndpoint
            secret_token = $SecretToken
            drop_pending_updates = $true
        }
        $tgResp = Invoke-RestMethod -Uri $setWebhookUrl -Method Post -Body ($body | ConvertTo-Json) -ContentType "application/json"
        if ($tgResp.ok) {
            Write-Host " [PASS] (Webhook set with secret token)" -ForegroundColor Green
        } else {
            Write-Host " [FAIL: $($tgResp.description)]" -ForegroundColor Red
        }
    } catch {
        Write-Host " [FAIL: $_]" -ForegroundColor Red
    }
}

if (-not $SkipScheduler) {
    Write-Host "[6b/6] Configuring Cloud Scheduler (06:00 UTC / 09:00 MSK)..." -NoNewline
    $cronEndpoint = "$serviceUrl/cron/daily-digest"
    $schedulerJobName = "moex-daily-digest"
    
    # Try updating or creating
    $existingJob = gcloud scheduler jobs describe $schedulerJobName --location $Region --project $Project 2>$null
    if ($existingJob) {
        gcloud scheduler jobs update http $schedulerJobName `
            --location $Region `
            --project $Project `
            --schedule="0 6 * * *" `
            --uri=$cronEndpoint `
            --http-method=POST 2>&1 | Out-Null
        Write-Host " [UPDATED]" -ForegroundColor Green
    } else {
        gcloud scheduler jobs create http $schedulerJobName `
            --location $Region `
            --project $Project `
            --schedule="0 6 * * *" `
            --uri=$cronEndpoint `
            --http-method=POST 2>&1 | Out-Null
        Write-Host " [CREATED]" -ForegroundColor Green
    }
}

Write-Host "`nRunning post-deployment sanity check..." -ForegroundColor Cyan
& (Join-Path $PSScriptRoot "sanity_check.ps1") -EndpointUrl $serviceUrl -SecretToken $SecretToken -BotToken $BotToken

Write-Host "`n=======================================================" -ForegroundColor Green
Write-Host "🎉 DEPLOYMENT COMPLETE!" -ForegroundColor Green
Write-Host "   Service URL:    $serviceUrl" -ForegroundColor Cyan
Write-Host "   Health Check:   $serviceUrl/health" -ForegroundColor Cyan
Write-Host "   Webhook:        $serviceUrl/webhook" -ForegroundColor Cyan
Write-Host "=======================================================`n" -ForegroundColor Green
