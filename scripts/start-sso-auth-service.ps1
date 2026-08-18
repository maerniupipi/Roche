# Start the Auth Service with the local SAML SSO integration environment.
#
# Prerequisites:
#   1) docker compose --env-file .env.sso-local -f docker-compose.sso-local.yml up -d
#   2) go build -o sso-auth-service.exe ./cmd/auth-service
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts/start-sso-auth-service.ps1
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

# 1) Load .env.sso-local (SAML_* / AUTH_* / secrets) into the process environment.
Get-Content ".env.sso-local" -Encoding UTF8 | ForEach-Object {
    $line = $_.Trim()
    if ($line -and -not $line.StartsWith("#") -and $line -match "^([^=]+)=(.*)$") {
        [Environment]::SetEnvironmentVariable($Matches[1].Trim(), $Matches[2].Trim(), "Process")
    }
}

# 2) Host-side overrides: DB/Redis live on the host mapped ports of the sso compose.
[Environment]::SetEnvironmentVariable("DB_HOST", "127.0.0.1", "Process")
[Environment]::SetEnvironmentVariable("DB_PORT", "5433", "Process")
[Environment]::SetEnvironmentVariable("REDIS_ADDR", "127.0.0.1:6380", "Process")

# 3) ★ 关键：Auth Service 签发的 access token 必须能被业务后端 (app:8080) 验签，
#    且 token 记录必须落在 app 同一个数据库里，否则 app 的 middleware.Auth 会以
#    "Unauthorized: invalid authentication" 拒绝所有请求（本地曾因 JWT_SECRET 与
#    DB 不一致导致 dashboard 全部 401）。
#    app (docker-compose.local-app.yml) 使用:
#      - JWT_SECRET=rochekap-local-jwt-secret
#      - SYSTEM_AES_KEY=rochekap-local-system-key-32byte
#      - DB = postgres-dev 宿主映射 127.0.0.1:5432
#      - REDIS = redis-dev 宿主映射 127.0.0.1:6379
#    因此这里强制覆盖为与 app 一致的取值（不随 .env.sso-local 漂移）。
[Environment]::SetEnvironmentVariable("JWT_SECRET", "rochekap-local-jwt-secret", "Process")
[Environment]::SetEnvironmentVariable("SYSTEM_AES_KEY", "rochekap-local-system-key-32byte", "Process")
[Environment]::SetEnvironmentVariable("DB_HOST", "127.0.0.1", "Process")
[Environment]::SetEnvironmentVariable("DB_PORT", "5432", "Process")
[Environment]::SetEnvironmentVariable("REDIS_ADDR", "127.0.0.1:6379", "Process")

# 3) Launch in the background with log redirection.
$stdout = Join-Path $Root "sso-auth-service.log"
$stderr = Join-Path $Root "sso-auth-service.err.log"
Start-Process -FilePath (Join-Path $Root "sso-auth-service.exe") -WorkingDirectory $Root `
    -RedirectStandardOutput $stdout -RedirectStandardError $stderr -WindowStyle Hidden

Start-Sleep -Seconds 3
Write-Host "Auth Service (SAML SSO) started. Logs:"
Write-Host "  stdout: $stdout"
Write-Host "  stderr: $stderr"
