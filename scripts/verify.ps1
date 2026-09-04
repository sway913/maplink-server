param(
    [ValidateSet('unit', 'integration', 'static', 'e2e', 'all')]
    [string]$Suite = 'all'
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $RepoRoot

function Require-Command([string]$Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "缺少必需命令: $Name"
    }
}

function Invoke-External([scriptblock]$Command) {
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "命令执行失败，退出码: $LASTEXITCODE"
    }
}

function Prepare-Web {
    Require-Command 'node'
    Require-Command 'npm.cmd'
    if (-not (Test-Path "$RepoRoot/web/node_modules")) {
        Push-Location "$RepoRoot/web"
        try { Invoke-External { npm.cmd ci } } finally { Pop-Location }
    }
}

function Test-Unit {
    Require-Command 'go'
    Prepare-Web
    Write-Host '== Unit tests =='
    Invoke-External { go test -race ./internal/auth ./internal/frp ./internal/version }
    Invoke-External { node --test tests/*.test.mjs }
    Push-Location "$RepoRoot/web"
    try { Invoke-External { npm.cmd test } } finally { Pop-Location }
}

function Test-Integration {
    Require-Command 'go'
    Write-Host '== Integration tests =='
    Invoke-External { go test -race ./internal/manager -run 'Test' -skip 'TestRemoteRelayAuthenticatesAndMovesFramesAndInputWithoutPersistence' -count=1 }
}

function Test-Static {
    Require-Command 'go'
    Require-Command 'git'
    Prepare-Web
    Write-Host '== Build and static checks =='
    Invoke-External { go vet ./... }
    New-Item -ItemType Directory -Force "$RepoRoot/bin" | Out-Null
    $env:CGO_ENABLED = '0'
    try { Invoke-External { go build -trimpath -o bin/frp-manager.exe ./cmd/frp-manager } }
    finally { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    Push-Location "$RepoRoot/web"
    try {
        Invoke-External { npm.cmd run lint }
        Invoke-External { npm.cmd run build:static }
    } finally { Pop-Location }

    $bash = Get-Command bash.exe -ErrorAction SilentlyContinue
    if ($bash) {
        Invoke-External { bash.exe -n scripts/bootstrap.sh scripts/install.sh scripts/verify.sh }
    }

    $privatePrefix = '-----BEGIN '
    $privateSuffix = '(OPENSSH|RSA|EC|DSA) PRIVATE KEY-----'
    $githubPrefix = 'github' + '_pat_'
    $ghPrefix = 'gh' + '[pousr]_'
    $pattern = "$privatePrefix$privateSuffix|$githubPrefix[A-Za-z0-9_]{20,}|$ghPrefix[A-Za-z0-9]{20,}"
    & git grep -nE -- $pattern
    if ($LASTEXITCODE -eq 0) { throw '检测到疑似真实凭据或私钥。' }
    if ($LASTEXITCODE -gt 1) { throw "敏感信息扫描执行失败: $LASTEXITCODE" }
}

function Test-CoreE2E {
    Require-Command 'go'
    Require-Command 'npx.cmd'
    Prepare-Web
    Write-Host '== Core E2E =='
    Invoke-External { go test ./internal/manager -run 'TestRemoteRelayAuthenticatesAndMovesFramesAndInputWithoutPersistence' -count=1 -v }
    Push-Location "$RepoRoot/web"
    try {
        Invoke-External { npm.cmd run build:static }
        Invoke-External { npx.cmd playwright test }
    } finally { Pop-Location }
}

switch ($Suite) {
    'unit' { Test-Unit }
    'integration' { Test-Integration }
    'static' { Test-Static }
    'e2e' { Test-CoreE2E }
    'all' {
        Test-Unit
        Test-Integration
        Test-Static
        Test-CoreE2E
    }
}

Write-Host "验证完成: $Suite"
