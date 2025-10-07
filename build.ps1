# Build script for Argus Configuration Framework (PowerShell)
# Supports reproducible builds and debug symbol preservation

param(
    [switch]$Strip,
    [switch]$Reproducible,
    [switch]$Verbose,
    [switch]$Help
)

$BinaryName = "argus"
$BuildDir = ".\bin"

# Show help
if ($Help) {
    Write-Host "Usage: .\build.ps1 [OPTIONS]"
    Write-Host "Options:"
    Write-Host "  -Strip        Strip debug symbols (production build)"
    Write-Host "  -Reproducible Enable reproducible builds (deterministic output)"
    Write-Host "  -Verbose      Enable verbose output" 
    Write-Host "  -Help         Show this help message"
    Write-Host ""
    Write-Host "By default, debug symbols are preserved for development."
    exit 0
}

# Create build directory
if (!(Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
}

# Determine build type
if ($Strip) {
    $BuildFlags = "-ldflags='-w -s'"
    Write-Host "Building $BinaryName (production - debug symbols stripped)..." -ForegroundColor Yellow
} else {
    $BuildFlags = ""
    Write-Host "Building $BinaryName (development - debug symbols preserved)..." -ForegroundColor Yellow
}

# Build command
$OutputPath = "$BuildDir\$BinaryName.exe"

if ($Verbose) {
    if ($BuildFlags) {
        Write-Host "go build $BuildFlags -o $OutputPath ."
    } else {
        Write-Host "go build -o $OutputPath ."
    }
}

try {
    # Reproducible builds with trimpath and buildid
    if ($Strip) {
        & go build -trimpath -ldflags="-w -s -buildid=" -o $OutputPath .
    } else {
        & go build -trimpath -ldflags="-buildid=" -o $OutputPath .
    }
    
    if ($LASTEXITCODE -ne 0) {
        throw "Build failed with exit code $LASTEXITCODE"
    }
    
    Write-Host "Build completed successfully!" -ForegroundColor Green
    Write-Host "Binary location: $OutputPath"
    
    # Show file size
    if (Test-Path $OutputPath) {
        $FileInfo = Get-Item $OutputPath
        $SizeKB = [math]::Round($FileInfo.Length / 1KB, 2)
        Write-Host "Binary size: $SizeKB KB"
        
        # Check for debug symbols (basic check)
        if ($Strip) {
            Write-Host "Debug symbols: Stripped" -ForegroundColor Yellow
        } else {
            Write-Host "Debug symbols: Present" -ForegroundColor Green
        }
    }
    
} catch {
    Write-Host "Build failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}