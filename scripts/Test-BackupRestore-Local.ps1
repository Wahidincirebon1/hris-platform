param(
  [Parameter(Mandatory = $true)]
  [string]$BackupPath
)

$ErrorActionPreference = 'Stop'
$resolvedBackup = (Resolve-Path -LiteralPath $BackupPath).Path
if (-not (Test-Path -LiteralPath $resolvedBackup -PathType Leaf)) {
  throw 'File backup tidak ditemukan.'
}
if ((Get-Item -LiteralPath $resolvedBackup).Length -eq 0) {
  throw 'File backup kosong.'
}

function Invoke-DockerChecked {
  param([string[]]$Arguments, [string]$FailureMessage)
  $output = & docker @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "$FailureMessage`n$($output -join [Environment]::NewLine)"
  }
  return $output
}

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 12)
$containerName = "hris-restore-test-$suffix"
$previousPassword = $env:POSTGRES_PASSWORD
$env:POSTGRES_PASSWORD = [Guid]::NewGuid().ToString('N')
$containerCreated = $false
try {
  Invoke-DockerChecked -Arguments @(
    'run', '--detach', '--rm', '--network', 'none', '--name', $containerName,
    '--env', 'POSTGRES_PASSWORD', '--env', 'POSTGRES_DB=hris_restore_test',
    '--mount', "type=bind,source=$resolvedBackup,target=/backup.dump,readonly",
    'postgres:16-alpine'
  ) -FailureMessage 'Container restore sementara tidak dapat dijalankan.' | Out-Null
  $containerCreated = $true

  $ready = $false
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    & docker exec $containerName pg_isready -U postgres -d hris_restore_test 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
      $ready = $true
      break
    }
    Start-Sleep -Seconds 2
  }
  if (-not $ready) {
    throw 'PostgreSQL sementara tidak siap dalam 60 detik.'
  }

  Invoke-DockerChecked -Arguments @(
    'exec', $containerName, 'pg_restore', '--exit-on-error', '--no-owner',
    '-U', 'postgres', '-d', 'hris_restore_test', '/backup.dump'
  ) -FailureMessage 'Restore backup pada container sementara gagal.' | Out-Null
  $result = Invoke-DockerChecked -Arguments @(
    'exec', $containerName, 'psql', '-U', 'postgres', '-d', 'hris_restore_test',
    '-v', 'ON_ERROR_STOP=1', '-Atc',
    "SELECT current_database(), to_regclass('public.schema_migrations') IS NOT NULL"
  ) -FailureMessage 'Validasi schema hasil restore gagal.'
  $resultText = $result -join "`n"
  Write-Output $resultText
  if ($resultText -notmatch 'hris_restore_test\|t') {
    throw 'Database restore selesai, tetapi tabel schema_migrations tidak ditemukan.'
  }
  Write-Output 'PASS: backup berhasil dipulihkan dan schema tervalidasi dalam container sementara tanpa jaringan atau volume persisten.'
} finally {
  if ($containerCreated) {
    & docker rm -f $containerName 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
      Write-Warning "Container uji $containerName tidak dapat dihapus otomatis."
    }
  }
  if ($null -eq $previousPassword) {
    Remove-Item Env:POSTGRES_PASSWORD -ErrorAction SilentlyContinue
  } else {
    $env:POSTGRES_PASSWORD = $previousPassword
  }
}
