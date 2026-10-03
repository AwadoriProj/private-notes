$ErrorActionPreference = "Stop"
$ListenHost = "0.0.0.0"
$Port = 8080
$Output = "capture/latest"
$Web = $false
$Positionals = [System.Collections.Generic.List[string]]::new()

for ($i = 0; $i -lt $args.Count; $i++) {
    switch -Regex ($args[$i].ToLowerInvariant()) {
        '^(--web|-web)$' { $Web = $true; continue }
        '^(--listen-host|-listenhost)$' {
            $i++
            if ($i -ge $args.Count) { throw "Missing value for $($args[$i - 1])" }
            $ListenHost = $args[$i]
            continue
        }
        '^(--listen-port|-port)$' {
            $i++
            if ($i -ge $args.Count) { throw "Missing value for $($args[$i - 1])" }
            $Port = [int]$args[$i]
            continue
        }
        '^(--output|-output)$' {
            $i++
            if ($i -ge $args.Count) { throw "Missing value for $($args[$i - 1])" }
            $Output = $args[$i]
            continue
        }
        default {
            if ($args[$i].StartsWith("-")) { throw "Unknown option: $($args[$i])" }
            $Positionals.Add($args[$i])
        }
    }
}

if ($Positionals.Count -gt 0) { $ListenHost = $Positionals[0] }
if ($Positionals.Count -gt 1) { $Port = [int]$Positionals[1] }
if ($Positionals.Count -gt 2) { $Output = $Positionals[2] }
if ($Positionals.Count -gt 3) { throw "Too many positional arguments" }

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$OutputPath = Join-Path $Root $Output
New-Item -ItemType Directory -Force -Path $OutputPath | Out-Null
$env:CAPTURE_OUTPUT = $OutputPath

if ($Web) {
    $ProxyCommand = "mitmweb"
} else {
    $ProxyCommand = "mitmproxy"
}

if (-not (Get-Command $ProxyCommand -ErrorAction SilentlyContinue)) {
    throw "$ProxyCommand is required. Install it with: py -m pip install mitmproxy"
}

Write-Host "Starting $ProxyCommand capture proxy at ${ListenHost}:$Port"
Write-Host "Use mitm.it on the test device to install and trust this proxy's CA certificate."
Write-Host "Captured endpoint JSON will be saved under $OutputPath"
Write-Host "All flows will be saved to $(Join-Path $OutputPath 'raw')"
Write-Host "After traffic is captured, run: py capture/generate_captured.py"
if ($Web) { Write-Host "mitmweb inspector: http://127.0.0.1:8081" }

Push-Location $Root
try {
    & $ProxyCommand --listen-host $ListenHost --listen-port $Port --save-stream-file (Join-Path $OutputPath "raw") -s (Join-Path $PSScriptRoot "capture_mitm.py")
}
finally {
    Pop-Location
}
