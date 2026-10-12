# Native Windows helper; compatible with Windows PowerShell 5.1 and PowerShell 7.
param([ValidateSet('deps', 'wasm', 'build', 'run', 'docker-build')][string]$Task = 'deps')
$ErrorActionPreference = 'Stop'

function Invoke-Go {
    & go @args
    if ($LASTEXITCODE -ne 0) { throw "go failed (exit $LASTEXITCODE)" }
}

function Get-Version {
    if (Get-Command git -ErrorAction SilentlyContinue) {
        # cmd handles expected git stderr without PowerShell 5.1 treating it as an error.
        $version = & cmd.exe /d /c 'git describe --tags --abbrev=0 2>nul'
        if ($LASTEXITCODE -eq 0) { return $version.Trim() }
        $version = & cmd.exe /d /c 'git rev-parse --short HEAD 2>nul'
        if ($LASTEXITCODE -eq 0) { return $version.Trim() }
    }
    return 'dev'
}

function Build-Wasm {
    $css = Get-Item -LiteralPath 'internal/app/static/pico.min.css'
    if ($css.Length -eq 0) { throw 'Bundled pico.min.css is empty' }
    $goRoot = Invoke-Go env GOROOT
    Copy-Item -LiteralPath (Join-Path $goRoot 'lib/wasm/wasm_exec.js') -Destination 'internal/app/static/wasm_exec.js'
    $oldOS, $oldArch, $oldCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
    try {
        $env:GOOS = 'js'; $env:GOARCH = 'wasm'; $env:CGO_ENABLED = '0'
        Invoke-Go build -trimpath '-ldflags=-s -w' -o internal/app/static/app.wasm ./cmd/wasm
    } finally {
        $env:GOOS = $oldOS; $env:GOARCH = $oldArch; $env:CGO_ENABLED = $oldCGO
    }
}

function Import-DotEnv {
    if (!(Test-Path -LiteralPath '.env')) { return }
    $lineNumber = 0
    foreach ($line in Get-Content -LiteralPath '.env' -Encoding UTF8) {
        $lineNumber++
        if ($line -match '^\s*(#|$)') { continue }
        if ($line -notmatch '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') {
            throw "Invalid .env assignment at line $lineNumber"
        }
        $name = $Matches[1]; $value = $Matches[2].Trim()
        if ($value.StartsWith('"') -or $value.StartsWith("'")) {
            if ($value.Length -lt 2 -or $value[$value.Length - 1] -ne $value[0]) {
                throw "Invalid .env quotes at line $lineNumber"
            }
            $value = $value.Substring(1, $value.Length - 2)
        } else {
            $value = ($value -replace '\s+#.*$', '').TrimEnd()
        }
        # Values are data: never execute or expand shell expressions from .env.
        [Environment]::SetEnvironmentVariable($name, $value, 'Process')
    }
}

Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    switch ($Task) {
        'deps' { Invoke-Go mod download; Invoke-Go mod verify }
        'wasm' { Build-Wasm }
        'build' {
            Build-Wasm
            New-Item -ItemType Directory -Force -Path bin | Out-Null
            $version = Get-Version
            $oldCGO = $env:CGO_ENABLED
            try {
                $env:CGO_ENABLED = '0'
                Invoke-Go build -trimpath "-ldflags=-s -w -X anonblossom/internal/web.Version=$version" -o bin/anonblossom.exe ./cmd/anonblossom
            } finally { $env:CGO_ENABLED = $oldCGO }
        }
        'run' {
            Build-Wasm
            Import-DotEnv
            $version = Get-Version
            Invoke-Go run "-ldflags=-X anonblossom/internal/web.Version=$version" ./cmd/anonblossom
        }
        'docker-build' {
            $version = Get-Version
            & docker build --build-arg "VERSION=$version" -t anonblossom:local .
            if ($LASTEXITCODE -ne 0) { throw "docker failed (exit $LASTEXITCODE)" }
        }
    }
} finally { Pop-Location }
