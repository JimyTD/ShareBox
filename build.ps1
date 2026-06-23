# ============================================================
# ShareBox build script
# ============================================================

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

# --- Go environment setup ---
$GoBin = "C:\Go125\go\bin"
$env:Path = "$GoBin;$env:Path"
$env:GOPATH = "$env:USERPROFILE\go"
$env:CGO_ENABLED = "0"

# --- Build Syncthing binary ---
function Build-Syncthing {
    Write-Host "[build] Building syncthing.exe (CGO_ENABLED=0)..." -ForegroundColor Cyan
    go build -o syncthing.exe ./cmd/syncthing
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }
    $size = [math]::Round((Get-Item syncthing.exe).Length / 1MB, 1)
    Write-Host "[build] Done! syncthing.exe ($size MB)" -ForegroundColor Green
}

# --- Generate GUI assets ---
function Generate-Assets {
    Write-Host "[gen] Generating GUI assets..." -ForegroundColor Cyan
    Push-Location lib\api\auto
    go generate
    Pop-Location
    Write-Host "[gen] Done! gui.files.go generated" -ForegroundColor Green
}

# --- Start server for testing ---
function Start-Server {
    $homeDir = ".\_test_home"
    if (-not (Test-Path syncthing.exe)) {
        Write-Host "[serve] Binary not found, building first..." -ForegroundColor Yellow
        Build-Syncthing
    }
    Write-Host "[serve] Starting syncthing (home=$homeDir)..." -ForegroundColor Cyan
    Start-Process -FilePath ".\syncthing.exe" -ArgumentList "serve", "--no-browser", "--home", $homeDir -NoNewWindow -PassThru
    Start-Sleep 2
    Write-Host "[serve] Web UI should be at http://127.0.0.1:4398" -ForegroundColor Green
    Write-Host "[serve] API key in $homeDir\config.xml" -ForegroundColor Green
}

# --- Clean build artifacts ---
function Clean-Build {
    Write-Host "[clean] Removing build artifacts..." -ForegroundColor Cyan
    Remove-Item syncthing.exe -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force _test_home -ErrorAction SilentlyContinue
    Write-Host "[clean] Done!" -ForegroundColor Green
}

# --- Full build pipeline ---
function Build-Full {
    Generate-Assets
    Build-Syncthing
}

# --- Dispatch ---
$cmd, $rest = $args
switch ($cmd) {
    "build"   { Build-Syncthing }
    "gen"     { Generate-Assets }
    "serve"   { Start-Server }
    "full"    { Build-Full }
    "clean"   { Clean-Build }
    "test"    {
        $env:LOGGER_DISCARD = 1
        go run build.go test @rest
    }
    "bench"   {
        $env:LOGGER_DISCARD = 1
        go run build.go bench @rest
    }
    default   {
        Write-Host @"
ShareBox build script - Usage:
  .\build.ps1 build   - Build syncthing.exe
  .\build.ps1 gen     - Generate GUI assets (gui.files.go)
  .\build.ps1 full    - Generate assets + build
  .\build.ps1 serve   - Build & start server for testing
  .\build.ps1 test    - Run tests
  .\build.ps1 bench   - Run benchmarks
  .\build.ps1 clean   - Remove build artifacts

Environment:
  Go:       $GoBin
  GOPATH:   $env:GOPATH
  CGO:      disabled
"@
    }
}
