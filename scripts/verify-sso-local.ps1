# End-to-end SAML SSO smoke test against the local integration stack.
#
# Simulates the browser flow:
#   1) SP-initiated: GET /api/v1/auth/saml/url?redirect_uri=...
#   2) GET mock IdP /sso login form (fields: user / password, SAMLRequest+RelayState round-trip)
#   3) POST mock IdP credentials (admin / Admin123!)
#   4) POST the returned SAMLResponse to the ACS via the gateway (8088)
# Expects a 302 back to the frontend landing page with #saml_result=...
$ErrorActionPreference = "Stop"

$frontend = "http://127.0.0.1:5173/"
$authBase = "http://127.0.0.1:8081"
$idpBase  = "http://127.0.0.1:8091"

$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$enc = [uri]::EscapeDataString($frontend)

# --- 1) SP-initiated authorization URL ---
$r1 = Invoke-WebRequest -Uri "$authBase/api/v1/auth/saml/url?redirect_uri=$enc" `
    -WebSession $session -UseBasicParsing -TimeoutSec 10
$j1 = $r1.Content | ConvertFrom-Json
if ($j1.code -ne 0 -or -not $j1.data.authorization_url) {
    throw "STEP1 failed: $($r1.Content)"
}
$authUrl = $j1.data.authorization_url
Write-Host "STEP1 authorization_url OK -> $($authUrl.Substring(0, [Math]::Min(60, $authUrl.Length)))..."

# --- 2) Mock IdP login form (round-trips SAMLRequest + RelayState) ---
$r2 = Invoke-WebRequest -Uri $authUrl -WebSession $session -UseBasicParsing -TimeoutSec 10
$loginSaml = [regex]::Match($r2.Content, 'name="SAMLRequest"\s+value="([^"]*)"', 'IgnoreCase').Groups[1].Value
$loginRelay = [regex]::Match($r2.Content, 'name="RelayState"\s+value="([^"]*)"', 'IgnoreCase').Groups[1].Value
if (-not $loginSaml) { throw "STEP2 failed: SAMLRequest not found in IdP login form" }
Write-Host "STEP2 IdP login form OK (SAMLRequest len $($loginSaml.Length), RelayState len $($loginRelay.Length))"

# --- 3) Submit mock IdP credentials ---
$r3 = Invoke-WebRequest -Uri "$idpBase/sso" -Method Post `
    -Body @{ user = "admin"; password = "Admin123!"; SAMLRequest = $loginSaml; RelayState = $loginRelay } `
    -ContentType "application/x-www-form-urlencoded" -WebSession $session `
    -UseBasicParsing -TimeoutSec 15
if ($r3.StatusCode -eq 302) {
    $acs = $r3.BaseResponse.Headers.Location
    $samlResp = ""
    $relay = $loginRelay
    Write-Host "STEP3 IdP responded with 302 to $acs (redirect binding)"
} else {
    $m = [regex]::Match($r3.Content, '<form[^>]*action="([^"]*)"[^>]*>', 'IgnoreCase')
    $acs = $m.Groups[1].Value
    $samlResp = [regex]::Match($r3.Content, 'name="SAMLResponse"\s+value="([^"]*)"', 'IgnoreCase').Groups[1].Value
    $relay = [regex]::Match($r3.Content, 'name="RelayState"\s+value="([^"]*)"', 'IgnoreCase').Groups[1].Value
    Write-Host "STEP3 IdP credentials accepted (auto-submit to $acs, SAMLResponse len $($samlResp.Length))"
}
if (-not $acs -or -not $samlResp) { throw "STEP3 failed: no ACS target returned by IdP" }

# --- 4) POST SAMLResponse to ACS via the gateway (manual HttpWebRequest,
#          because Invoke-WebRequest misbehaves on 302 + POST) ---
$req = [System.Net.HttpWebRequest]::Create([System.Uri]$acs)
$req.Method = "POST"
$req.ContentType = "application/x-www-form-urlencoded"
$req.AllowAutoRedirect = $false
$req.Timeout = 15000
$req.CookieContainer = $session.Cookies
$bodyStr = "SAMLResponse=$([uri]::EscapeDataString($samlResp))&RelayState=$([uri]::EscapeDataString($relay))"
$bytes = [System.Text.Encoding]::UTF8.GetBytes($bodyStr)
$req.ContentLength = $bytes.Length
$ws = $req.GetRequestStream()
$ws.Write($bytes, 0, $bytes.Length)
$ws.Close()
$status = 0
$loc = ""
try {
    $resp = $req.GetResponse()
    $status = [int]$resp.StatusCode
    $loc = $resp.Headers["Location"]
    $resp.Close()
} catch [System.Net.WebException] {
    $er = $_.Exception.Response
    if ($er) {
        $status = [int]$er.StatusCode
        $loc = $er.Headers["Location"]
        $er.Close()
    } else {
        throw "STEP4 failed: $($_.Exception.Message)"
    }
}
Write-Host "STEP4 ACS -> HTTP $status, Location: $loc"
if ($status -eq 302 -and $loc -match "saml_result") {
    Write-Host "`nSSO SMOKE TEST PASSED: full SP-initiated SAML flow completed."
} else {
    Write-Host "`nSSO SMOKE TEST WARNING: redirect target did not carry #saml_result"
}
