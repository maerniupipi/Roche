param(
    [ValidateSet("check", "app-host", "app-docker", "down-docker", "logs-docker")]
    [string]$Command = "check",
    [string]$EnvFile = ".env.remote-dev"
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$EnvPath = if ([System.IO.Path]::IsPathRooted($EnvFile)) {
    $EnvFile
} else {
    Join-Path $ProjectRoot $EnvFile
}

function Import-DotEnv([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "Environment file not found: $Path. Copy .env.remote-dev.example to .env.remote-dev first."
    }

    foreach ($line in Get-Content -LiteralPath $Path -Encoding UTF8) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith("#")) { continue }
        $separator = $trimmed.IndexOf("=")
        if ($separator -lt 1) { continue }
        $name = $trimmed.Substring(0, $separator).Trim()
        $value = $trimmed.Substring($separator + 1).Trim()
        if ($value.Length -ge 2 -and (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'")))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        [Environment]::SetEnvironmentVariable($name, $value, "Process")
    }
}

function Test-TcpPort([string]$Name, [string]$HostName, [int]$Port) {
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $async = $client.BeginConnect($HostName, $Port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne(3000)) {
            Write-Host "[FAIL] $Name $HostName`:$Port (timeout)" -ForegroundColor Red
            return $false
        }
        $client.EndConnect($async)
        Write-Host "[ OK ] $Name $HostName`:$Port" -ForegroundColor Green
        return $true
    } catch {
        Write-Host "[FAIL] $Name $HostName`:$Port ($($_.Exception.Message))" -ForegroundColor Red
        return $false
    } finally {
        $client.Dispose()
    }
}

function Test-RemoteInfrastructure {
    $hostName = $env:DEV_REMOTE_HOST
    if (-not $hostName) { throw "DEV_REMOTE_HOST is missing in $EnvPath" }
    $checks = @(
        @("PostgreSQL", [int]$env:DB_PORT),
        @("Redis", 6379),
        @("Milvus", 19530),
        @("DocReader", 50051),
        @("Neo4j", 7687),
        @("MinIO", 9000)
    )
    if ($env:LANGFUSE_ENABLED -eq "true") {
        $checks += ,@("Langfuse", 3000)
    }
    $allOk = $true
    foreach ($check in $checks) {
        if (-not (Test-TcpPort $check[0] $hostName $check[1])) { $allOk = $false }
    }
    if (-not $allOk) { throw "One or more remote development services are unreachable." }
}

Import-DotEnv $EnvPath
Push-Location $ProjectRoot
try {
    switch ($Command) {
        "check" {
            Test-RemoteInfrastructure
        }
        "app-host" {
            Test-RemoteInfrastructure
            $gitBashCandidates = [System.Collections.Generic.List[string]]@(
                "C:\Program Files\Git\bin\bash.exe",
                "C:\Program Files\Git\usr\bin\bash.exe"
            )
            $gitCommand = Get-Command git -ErrorAction SilentlyContinue
            if ($gitCommand) {
                $gitRoot = Split-Path -Parent (Split-Path -Parent $gitCommand.Source)
                $gitBashCandidates.Insert(0, (Join-Path $gitRoot "bin\bash.exe"))
            }
            $bash = $gitBashCandidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
            if (-not $bash) { throw "Git Bash was not found." }
            $env:DEV_ENV_FILE = $EnvFile.Replace('\', '/')
            & $bash "./scripts/dev.sh" app
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
        "app-docker" {
            Test-RemoteInfrastructure
            $env:REMOTE_DEV_ENV_FILE = $EnvFile
            docker compose --env-file $EnvPath -f docker-compose.remote-dev.yml up --build app
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
        "down-docker" {
            $env:REMOTE_DEV_ENV_FILE = $EnvFile
            docker compose --env-file $EnvPath -f docker-compose.remote-dev.yml down
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
        "logs-docker" {
            $env:REMOTE_DEV_ENV_FILE = $EnvFile
            docker compose --env-file $EnvPath -f docker-compose.remote-dev.yml logs -f app
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        }
    }
} finally {
    Pop-Location
}
