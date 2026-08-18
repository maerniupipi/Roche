# RocheKAP Audit Log Test v3
param([int]$Port = 8080)

$Base = "http://localhost:$Port/api/v1"
$ErrorActionPreference = "Continue"

$TS = Get-Date -Format 'HHmmss'
$EMAIL    = "audit_$TS@test.local"
$PASSWORD = "Test@123456"
$USERNAME = "audit_$TS"
$Token = ""

function Call($Method, $Url, $Body, $Tok) {
    $h = @{ "Content-Type" = "application/json" }
    if ($Tok) { $h["Authorization"] = "Bearer $Tok" }
    $p = @{ Method = $Method; Uri = $Url; Headers = $h }
    if ($Body) { $p["Body"] = ($Body | ConvertTo-Json -Depth 10 -Compress) }
    try {
        return Invoke-RestMethod @p
    } catch {
        $sc = 0
        $bodyText = ""
        try { $sc = $_.Exception.Response.StatusCode.value__ } catch { $sc = 0 }
        try {
            $sr = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $bodyText = $sr.ReadToEnd()
            $sr.Close()
        } catch { $bodyText = $_.Exception.Message }
        return @{ _err = $true; _status = $sc; _body = $bodyText }
    }
}

function Ok($t) { Write-Host "  [OK] $t" -ForegroundColor Green }
function Fail($t) { Write-Host "  [FAIL] $t" -ForegroundColor Red }
function Info($t) { Write-Host "  [INFO] $t" -ForegroundColor Gray }
function Step($t) { Write-Host "`n--- $t ---" -ForegroundColor Yellow }

function Unwrap($r) {
    if ($r.data) { return $r.data } else { return $r }
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host " RocheKAP Audit Log Test" -ForegroundColor Cyan
Write-Host " Base: $Base" -ForegroundColor Gray
Write-Host " User: $EMAIL" -ForegroundColor Gray
Write-Host "========================================" -ForegroundColor Cyan

# 1. Registration config
Step "Check registration config"
$cfg = Call -Method GET -Url "$Base/auth/registration/config"
$cfgData = Unwrap $cfg
Ok "Registration enabled=$($cfgData.enabled), roles=$($cfgData.roles -join ',')"

# 2. Register as system_admin
Step "Register as system_admin"
$reg = Call -Method POST -Url "$Base/auth/register" -Body @{
    username = $USERNAME
    email = $EMAIL
    password = $PASSWORD
    role = "system_admin"
}
$regData = Unwrap $reg
if ($regData.success) { Ok "Registered: $EMAIL (role=system_admin)" }
elseif ($regData.message -match "already|exists") { Ok "Already exists" }
else { Info "Response: $($reg | ConvertTo-Json -Depth 2 -Compress)" }

# 3. Login success
Step "Login (correct password)"
$login = Call -Method POST -Url "$Base/auth/login" -Body @{
    email = $EMAIL
    password = $PASSWORD
}
$loginData = Unwrap $login
if ($loginData.success -and $loginData.token) {
    $Token = $loginData.token
    Ok "Token: $($Token.Substring(0, 25))..."
    Ok "is_system_admin=$($loginData.user.is_system_admin)"
} else {
    Fail "Login failed: $($login | ConvertTo-Json -Depth 2 -Compress)"
    exit 1
}

# 4. Login failed
Step "Login (wrong password)"
$bad = Call -Method POST -Url "$Base/auth/login" -Body @{
    email = $EMAIL
    password = "WrongPass999"
}
if ($bad._err -and $bad._status -eq 401) { Ok "Rejected 401" }
elseif ($bad.data -and -not $bad.data.success) { Ok "Login denied" }
else { Info "Unexpected: status=$($bad._status)" }

# 5. Logout
Step "Logout"
$logout = Call -Method POST -Url "$Base/auth/logout" -Tok $Token
$loData = Unwrap $logout
if ($logout._err) { Info "Logout response: $($logout._status)" }
elseif ($loData.success -or $loData.message) { Ok "Logout: $($loData.message)" }
else { Info "Logout: $($logout | ConvertTo-Json -Depth 2 -Compress)" }

# 6. Token invalid after logout
Step "Token invalid after logout"
Start-Sleep -Seconds 1
$me = Call -Method GET -Url "$Base/auth/me" -Tok $Token
if ($me._err -and $me._status -eq 401) { Ok "Token invalidated (401)" }
else { Info "Token still valid?" }

# 7. Re-login
Step "Re-login"
$login2 = Call -Method POST -Url "$Base/auth/login" -Body @{
    email = $EMAIL
    password = $PASSWORD
}
$login2Data = Unwrap $login2
if ($login2Data.token) {
    $Token = $login2Data.token
    Ok "Re-login OK"
} else { Fail "Re-login failed" }

# 8. Knowledge bases
Step "Knowledge bases"
$kb = Call -Method GET -Url "$Base/knowledge-bases" -Tok $Token
$kbData = Unwrap $kb
if ($kbData.GetType().IsArray) {
    if ($kbData.Count -gt 0) {
        Ok "Count: $($kbData.Count), First: $($kbData[0].name) id=$($kbData[0].id)"
    } else { Info "Count: 0 (empty)" }
} elseif ($kbData.data -and $kbData.data.Count -gt 0) {
    Ok "Count: $($kbData.data.Count), First: $($kbData.data[0].name) id=$($kbData.data[0].id)"
} else { Info "No KBs or unexpected format" }

# 9. Query audit logs (admin)
Step "Query audit logs"
Start-Sleep -Seconds 2
$audit = Call -Method GET -Url "$Base/system/admin/audit-log?limit=20" -Tok $Token
$auditData = Unwrap $audit
if ($audit._err) { Fail "Audit query: $($audit._status) - $($audit._body)" }
elseif ($auditData.GetType().IsArray) {
    Ok "Records: $($auditData.Count)"
    foreach ($r in $auditData) {
        $c = if ($r.outcome -eq "denied") { "Red" } else { "Gray" }
        Write-Host "  #$($r.id) [$($r.created_at)] $($r.action) [$($r.outcome)]" -ForegroundColor $c
    }
} elseif ($auditData.data) {
    Ok "Records: $($auditData.data.Count)"
    foreach ($r in $auditData.data) {
        $c = if ($r.outcome -eq "denied") { "Red" } else { "Gray" }
        Write-Host "  #$($r.id) [$($r.created_at)] $($r.action) [$($r.outcome)]" -ForegroundColor $c
    }
} else {
    Info "Unexpected format"
    $audit | ConvertTo-Json -Depth 2
}

# 10. DB query
Step "Direct DB query"
docker exec roche-kap-postgres-dev psql -U postgres -d RocheKAP -c "SELECT count(*) AS total FROM audit_logs;" -c "SELECT id, action, outcome, created_at FROM audit_logs ORDER BY id DESC LIMIT 10;" 2>&1

Write-Host "`n========================================" -ForegroundColor Cyan
Write-Host " Test Complete!" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
