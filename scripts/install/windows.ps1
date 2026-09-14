# Interactive installer for token-usage-service (Windows).
# Builds binaries, configures .env, runs migration, and optionally installs
# Scheduled Tasks for ingestion and the dashboard.
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$RepoDir = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$BinDir = Join-Path $RepoDir 'bin'
$EnvFile = Join-Path $RepoDir '.env'
$AllVendors = @('claude-code', 'codex', 'opencode', 'github-copilot', 'gemini', 'antigravity', 'cursor', 'cursor-usage')

function Write-Step([string]$Text) {
    Write-Host $Text -ForegroundColor Cyan
}

function Read-Default([string]$Prompt, [string]$Default) {
    $Value = Read-Host "$Prompt [$Default]"
    if ([string]::IsNullOrWhiteSpace($Value)) { return $Default }
    return $Value
}

Write-Host 'token-usage-service installer for Windows' -ForegroundColor Cyan
Write-Step '1/6 Windows + Go toolchain'
if (-not $IsWindows -and $PSVersionTable.PSEdition -eq 'Core') {
    throw 'This installer requires Windows.'
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'Go not found. Install Go 1.25+ with: winget install GoLang.Go; then reopen PowerShell.'
}
Write-Host "  found: $(go version)"

Write-Step '2/6 Choose adapters (components) to enable'
$Selected = foreach ($Vendor in $AllVendors) {
    $Reply = Read-Host "  enable ${Vendor}? [Y/n]"
    if ($Reply -notmatch '^[nN]') { $Vendor }
}
if ($Selected.Count -eq 0) {
    Write-Warning 'No adapters selected; enabling all.'
    $Selected = $AllVendors
}

Write-Step '3/6 Configuration'
$DatabaseUrl = Read-Default '  DATABASE_URL' 'postgres://USER:PASSWORD@localhost:5432/token_usage?sslmode=disable'
$HostId = Read-Default '  HOST_ID' $env:COMPUTERNAME
$EnvLines = @(
    "DATABASE_URL=$DatabaseUrl"
    "HOST_ID=$HostId"
    "ENABLED_VENDORS=$($Selected -join ',')"
    'SINK=postgres'
)
[System.IO.File]::WriteAllLines($EnvFile, $EnvLines, [System.Text.UTF8Encoding]::new($false))
Write-Host "  wrote $EnvFile"

Write-Step '4/6 Build'
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Push-Location $RepoDir
try {
    go build -o (Join-Path $BinDir 'token-usage-ingest.exe') ./cmd/ingest
    if ($LASTEXITCODE -ne 0) { throw 'Ingest build failed.' }
    go build -o (Join-Path $BinDir 'token-usage-loadworkstyle.exe') ./cmd/loadworkstyle
    if ($LASTEXITCODE -ne 0) { throw 'Loadworkstyle build failed.' }
    go build -o (Join-Path $BinDir 'token-usage-dashboard.exe') ./cmd/dashboard
    if ($LASTEXITCODE -ne 0) { throw 'Dashboard build failed.' }
} finally {
    Pop-Location
}
Write-Host "  built binaries in $BinDir"

Write-Step '5/6 Validate + migrate + first ingest'
$IngestExe = Join-Path $BinDir 'token-usage-ingest.exe'
$LoadworkstyleExe = Join-Path $BinDir 'token-usage-loadworkstyle.exe'
$WorkstyleArgs = @(
    "claude_history=$env:USERPROFILE\.claude\history.jsonl"
    "claude_caveman_history=$env:USERPROFILE\.claude\.caveman-history.jsonl"
    "codex_history=$env:USERPROFILE\.codex\history.jsonl"
    "codex_session_index=$env:USERPROFILE\.codex\session_index.jsonl"
)
Push-Location $RepoDir
try {
    & $IngestExe validate-config
    if ($LASTEXITCODE -ne 0) { throw 'Configuration validation failed.' }
    & $IngestExe migrate
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed.' }
    & $IngestExe ingest
    if ($LASTEXITCODE -ne 0) { throw 'First ingest failed.' }
    & $LoadworkstyleExe @WorkstyleArgs
    if ($LASTEXITCODE -ne 0) { throw 'First workstyle-log load failed.' }
} finally {
    Pop-Location
}

Write-Step '6/6 Scheduling'
$Reply = Read-Host '  install Scheduled Task for ingest (every 30m)? [Y/n]'
if ($Reply -notmatch '^[nN]') {
    $IngestAction = New-ScheduledTaskAction -Execute $IngestExe -Argument 'ingest' -WorkingDirectory $RepoDir
    $WorkstyleAction = New-ScheduledTaskAction -Execute $LoadworkstyleExe -Argument ($WorkstyleArgs -join ' ') -WorkingDirectory $RepoDir
    $Trigger = New-ScheduledTaskTrigger -Daily -At 12am
    $Repetition = (New-ScheduledTaskTrigger -Once -At (Get-Date) `
        -RepetitionInterval (New-TimeSpan -Minutes 30) `
        -RepetitionDuration (New-TimeSpan -Days 3650)).Repetition
    $Trigger.Repetition = $Repetition
    $Settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew
    Register-ScheduledTask -TaskName 'Token Usage Ingest' -Action $IngestAction, $WorkstyleAction -Trigger $Trigger `
        -Settings $Settings -RunLevel Limited -Force `
        -Description 'Collects local AI token usage every 30 minutes' | Out-Null
    Start-ScheduledTask -TaskName 'Token Usage Ingest'
    Write-Host '  ingest task installed and started'
}

$Reply = Read-Host '  install dashboard as a logon Scheduled Task? [y/N]'
if ($Reply -match '^[yY]') {
    $DashboardExe = Join-Path $BinDir 'token-usage-dashboard.exe'
    $Action = New-ScheduledTaskAction -Execute $DashboardExe -WorkingDirectory $RepoDir
    $Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $Settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew
    Register-ScheduledTask -TaskName 'Token Usage Dashboard' -Action $Action -Trigger $Trigger `
        -Settings $Settings -RunLevel Limited -Force `
        -Description 'Runs the local token usage dashboard at logon' | Out-Null
    Start-ScheduledTask -TaskName 'Token Usage Dashboard'
    Write-Host '  dashboard running: http://127.0.0.1:8080'
}

Write-Host 'Done.' -ForegroundColor Cyan
Write-Host "  ingest:        $IngestExe ingest"
Write-Host "  loadworkstyle: $LoadworkstyleExe"
Write-Host "  dashboard:     $(Join-Path $BinDir 'token-usage-dashboard.exe')"
Write-Host "  config:    $EnvFile"
