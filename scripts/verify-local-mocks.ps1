param(
    [string]$MockSAMLBaseUrl = "http://127.0.0.1:8091",
    [string]$GatewayBaseUrl = "http://127.0.0.1:8088",
    [string]$FrontendBaseUrl = "http://127.0.0.1:5173"
)

$ErrorActionPreference = "Stop"

function Get-EnvelopeData {
    param([object]$Response)

    if ($null -ne $Response.data) {
        return $Response.data
    }
    return $Response
}

$health = Invoke-RestMethod "$MockSAMLBaseUrl/healthz"
if ([string]$health -ne "ok") {
    throw "Mock SAML IdP health check failed."
}

$registration = Get-EnvelopeData (Invoke-RestMethod "$GatewayBaseUrl/api/v1/auth/registration/config")
if (-not $registration.enabled) {
    throw "Local email registration is disabled. Restart the backend after loading .env.local."
}
if (-not $registration.role_selection_enabled) {
    throw "Development registration role selection is disabled."
}

$saml = Get-EnvelopeData (Invoke-RestMethod "$GatewayBaseUrl/api/v1/auth/saml/config")
if (-not $saml.enabled) {
    throw "Mock SAML is not enabled in the Auth Service."
}

$redirectUri = "$FrontendBaseUrl/"
$encodedRedirectUri = [Uri]::EscapeDataString($redirectUri)
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
$authorization = Get-EnvelopeData (Invoke-RestMethod `
    "$GatewayBaseUrl/api/v1/auth/saml/url?redirect_uri=$encodedRedirectUri" `
    -WebSession $session
)
if (-not $authorization.authorization_url) {
    throw "The Auth Service did not return a Mock SAML authorization URL."
}

$authorizationUri = [Uri]$authorization.authorization_url
if ($authorizationUri.Host -ne "127.0.0.1" -or $authorizationUri.Port -ne 8091) {
    throw "Authorization URL does not point to the local Mock SAML IdP: $authorizationUri"
}

Write-Host "Email registration: OK (roles: $($registration.roles -join ', '))"
Write-Host "Mock SAML IdP health: OK"
Write-Host "Mock SAML backend configuration: OK ($($saml.provider_display_name))"
Write-Host "Mock SAML authorization redirect: OK"
Write-Host "Workday mock: configured through WORKDAY_PROVIDER=mock."
