# Uso: .\testing\run.ps1 <script>
# Ejemplos:
#   .\testing\run.ps1 api-smoke
#   .\testing\run.ps1 hls-vod
#   .\testing\run.ps1 hls-live

param(
    [Parameter(Mandatory)][ValidateSet('api-smoke','hls-vod','hls-live')]
    [string]$Script
)

$envFile = "$PSScriptRoot\.env"
if (-not (Test-Path $envFile)) {
    Write-Error "No se encontro $envFile. Copia .env.example a .env y completa los valores."
    exit 1
}

# Cargar variables del .env
Get-Content $envFile | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)=(.*)$') {
        $key = $Matches[1].Trim()
        $val = $Matches[2].Trim()
        [System.Environment]::SetEnvironmentVariable($key, $val, 'Process')
    }
}

# Validaciones especificas por script
if ($Script -eq 'hls-vod' -and -not $env:VIDEO_ID) {
    Write-Error "VIDEO_ID no esta definido en .env"
    exit 1
}
if ($Script -eq 'hls-live' -and -not $env:STREAM_ID) {
    Write-Error "STREAM_ID no esta definido en .env"
    exit 1
}

$scriptPath = "$PSScriptRoot\$Script.js"
Write-Host "Corriendo $Script (VUS=$env:VUS, DURATION=$env:DURATION, BASE_URL=$env:BASE_URL)" -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path "$PSScriptRoot\reports" | Out-Null

k6 run $scriptPath
