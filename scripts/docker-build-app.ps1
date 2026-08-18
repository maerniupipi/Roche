# ============================================================
# Precompile Go binary + build thin Docker image (Windows PowerShell)
#
# Strategy:
#   1. Compile inside a Linux container (docker run), mounting the host
#      Go build cache / module cache / duckdb cache for fast incremental builds
#   2. Package the prebuilt binary with the thin Dockerfile.app.fast (seconds)
#
# Usage (from repo root):
#   .\scripts\docker-build-app.ps1
#   $env:CACHE_DIR = "D:\cache\rochekap"; .\scripts\docker-build-app.ps1
#
# Then rebuild the app quickly:
#   docker compose --env-file .env.local -f docker-compose.local.yml -f docker-compose.local-app.yml up -d --build
# ============================================================
# Continue: native tools (docker/go/make) write progress to stderr; if we set
# "Stop", PowerShell 5.1 treats each stderr line as a terminating error.
# We check $LASTEXITCODE manually and throw on real failures instead.
$ErrorActionPreference = "Continue"

$ProjectDir = Split-Path $PSScriptRoot -Parent
Set-Location $ProjectDir
$CacheDir   = if ($env:CACHE_DIR) { $env:CACHE_DIR } else { Join-Path $env:TEMP "rochekap-build-cache" }
$BinaryName = "RocheKAP"
$BuilderImage = "rochekap/go-builder:local"

Write-Host "ProjectDir : $ProjectDir"
Write-Host "CacheDir   : $CacheDir"

foreach ($sub in @("go-build", "go-mod", "go-bin", "duckdb")) {
    New-Item -ItemType Directory -Force -Path (Join-Path $CacheDir $sub) | Out-Null
}

# ---- Build builder image once (apt deps + migrate preinstalled, layer-cached) ----
$builderExists = docker images -q $BuilderImage 2>$null
if (-not $builderExists) {
    Write-Host "=== Building builder image (one-time) ==="
    docker build --platform linux/amd64 -f docker/Dockerfile.builder -t $BuilderImage $ProjectDir
    if ($LASTEXITCODE -ne 0) { throw "Builder image build failed" }
} else {
    Write-Host "=== Reusing existing builder image ==="
}

# ---- Extract migrate binary from builder image ----
$migrateOut = Join-Path $CacheDir "go-bin\migrate"
if (-not (Test-Path $migrateOut)) {
    Write-Host "=== Extracting migrate ==="
    docker create --name tmp-builder-extract $BuilderImage 2>$null | Out-Null
    docker cp "tmp-builder-extract:/go/bin/migrate" $migrateOut
    docker rm tmp-builder-extract 2>$null | Out-Null
}

# ---- Incremental compile inside container with host cache mounts ----
Write-Host "=== Compiling (incremental) ==="
docker run --rm --platform linux/amd64 `
    -v "${ProjectDir}:/app" `
    -v "${CacheDir}\go-build:/root/.cache/go-build" `
    -v "${CacheDir}\go-mod:/go/pkg/mod" `
    -v "${CacheDir}\duckdb:/root/.duckdb" `
    -w /app `
    -e "GOPROXY=https://goproxy.cn,direct" `
    -e "GOSUMDB=off" `
    $BuilderImage bash -c "
        git config --global --add safe.directory /app
        go mod download
        go run cmd/download/duckdb/duckdb.go 2>/dev/null || true
        cp -r /go/pkg/mod/github.com/yanyiwu/ /app/yanyiwu/ 2>/dev/null || true
        make build-prod
    "
if ($LASTEXITCODE -ne 0) { throw "Compile inside container failed" }

# ---- Copy artifacts to repo root for Dockerfile.app.fast ----
Copy-Item $migrateOut (Join-Path $ProjectDir "migrate") -Force
$duckdbSrc = Join-Path $CacheDir "duckdb"
if ((Test-Path $duckdbSrc) -and (Get-ChildItem $duckdbSrc | Measure-Object).Count -gt 0) {
    Copy-Item $duckdbSrc (Join-Path $ProjectDir ".duckdb") -Recurse -Force
}
Write-Host "=== Build done: $(Join-Path $ProjectDir $BinaryName) ==="

# ---- Build thin app image (seconds, COPY prebuilt artifacts only) ----
Write-Host "=== Building thin app image ==="
docker build --platform linux/amd64 -f docker/Dockerfile.app.fast -t rochekap-app:local $ProjectDir
if ($LASTEXITCODE -ne 0) { throw "App image build failed" }
Write-Host "=== App image build complete ==="
