$ErrorActionPreference = "Stop"
$ListenHost = "0.0.0.0"
$Port = 8080
$Output = "capture/latest"
$Web = $false
$WireGuard = $false
$WireGuardPort = 51820
$Positionals = [System.Collections.Generic.List[string]]::new()

for ($i = 0; $i -lt $args.Count; $i++) {
    switch -Regex ($args[$i].ToLowerInvariant()) {
        '^(--web|-web)$' { $Web = $true; continue }
        '^(--wireguard|-wireguard)$' { $WireGuard = $true; continue }
        '^(--wireguard-port|-wireguardport)$' {
            $i++
            if ($i -ge $args.Count) { throw "Missing value for $($args[$i - 1])" }
            $WireGuardPort = [int]$args[$i]
            continue
        }
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

$ProxyArgs = @()
if ($ListenHost -ne "0.0.0.0") { $ProxyArgs += @("--listen-host", $ListenHost) }
if ($WireGuard) {
    $ProxyArgs += @("--mode", "regular@$Port", "--mode", "wireguard@$WireGuardPort")
} else {
    $ProxyArgs += @("--listen-port", "$Port")
}
$ProxyArgs += @("--save-stream-file", (Join-Path $OutputPath "raw"), "-s", (Join-Path $PSScriptRoot "capture_mitm.py"))

Write-Host "Starting $ProxyCommand capture proxy at ${ListenHost}:$Port"
if ($WireGuard) {
    Write-Host "WireGuard is enabled on UDP port $WireGuardPort. Scan the QR code in mitmweb (use --web), or import the config printed by mitmproxy."
    Write-Host "If the Endpoint in the config is not this computer's LAN IP, edit it in the WireGuard app."
}
Write-Host "Use mitm.it on the test device to install and trust this proxy's CA certificate."
Write-Host "Captured endpoint JSON will be saved under $OutputPath"
Write-Host "All flows will be saved to $(Join-Path $OutputPath 'raw')"
Write-Host "The upstream asset Authorization header will be saved to $(Join-Path $OutputPath 'upstream_authorization.txt') after a successful /asset/ or /master/ request."
Write-Host "After traffic is captured, run: py capture/generate_captured.py"
if ($Web) { Write-Host "mitmweb inspector: http://127.0.0.1:8081" }

Push-Location $Root
try {
    & $ProxyCommand @ProxyArgs
}
finally {
    Pop-Location
}
