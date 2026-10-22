$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$envPath = Join-Path $projectRoot '.env'

function New-RandomSecret {
  $bytes = New-Object byte[] 48
  $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
  try {
    $generator.GetBytes($bytes)
  } finally {
    $generator.Dispose()
  }
  return [Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
}

if (-not (Test-Path -LiteralPath $envPath)) {
  $content = "POSTGRES_PASSWORD=$(New-RandomSecret)`r`nJWT_SECRET=$(New-RandomSecret)`r`nCOOKIE_SECURE=false`r`n"
  [System.IO.File]::WriteAllText($envPath, $content, [System.Text.UTF8Encoding]::new($false))
} else {
  $hasJWTSecret = $false
  $hasPostgresPassword = $false
  foreach ($line in [System.IO.File]::ReadLines($envPath)) {
    if ($line.StartsWith('JWT_SECRET=')) { $hasJWTSecret = $true }
    if ($line.StartsWith('POSTGRES_PASSWORD=')) { $hasPostgresPassword = $true }
  }
  if (-not $hasPostgresPassword) {
    throw '.env exists but does not define POSTGRES_PASSWORD; no changes were made.'
  }
  if (-not $hasJWTSecret) {
    [System.IO.File]::AppendAllText($envPath, "JWT_SECRET=$(New-RandomSecret)`r`nCOOKIE_SECURE=false`r`n")
  }
}
Write-Output 'Local secrets are configured in .env and were not displayed.'
