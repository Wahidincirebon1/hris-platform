param(
  [string]$BackupDirectory = (Join-Path $env:LOCALAPPDATA 'HRIS-Platform\Backups')
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $projectRoot 'docker-compose.yml'
$timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 8)
$remotePath = "/tmp/hris-platform-$suffix.dump"
$backupPath = Join-Path $BackupDirectory "hris-platform-$timestamp-$suffix.dump"
$partialPath = "$backupPath.partial"

function Invoke-DockerChecked {
  param([string[]]$Arguments, [string]$FailureMessage)
  $output = & docker @Arguments 2>&1
  if ($LASTEXITCODE -ne 0) {
    throw "$FailureMessage`n$($output -join [Environment]::NewLine)"
  }
  return $output
}

New-Item -ItemType Directory -Path $BackupDirectory -Force | Out-Null
Invoke-DockerChecked -Arguments @('info', '--format', '{{.ServerVersion}}') -FailureMessage 'Docker daemon tidak siap.' | Out-Null
$container = (Invoke-DockerChecked -Arguments @('compose', '-f', $composeFile, 'ps', '-q', 'db') -FailureMessage 'Container database HRIS tidak dapat ditemukan.').Trim()
if (-not $container) {
  throw 'Container database HRIS belum berjalan.'
}

$createdRemoteDump = $false
try {
  $dumpCommand = 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f ' + $remotePath
  Invoke-DockerChecked -Arguments @('compose', '-f', $composeFile, 'exec', '-T', 'db', 'sh', '-c', $dumpCommand) -FailureMessage 'Backup PostgreSQL gagal.' | Out-Null
  $createdRemoteDump = $true
  Invoke-DockerChecked -Arguments @('compose', '-f', $composeFile, 'exec', '-T', 'db', 'pg_restore', '--list', $remotePath) -FailureMessage 'Arsip backup tidak valid.' | Out-Null
  Invoke-DockerChecked -Arguments @('cp', "${container}:$remotePath", $partialPath) -FailureMessage 'Arsip backup tidak dapat disalin ke komputer ini.' | Out-Null
  if (-not (Test-Path -LiteralPath $partialPath) -or (Get-Item -LiteralPath $partialPath).Length -eq 0) {
    throw 'Arsip backup lokal kosong atau tidak ditemukan.'
  }
  Move-Item -LiteralPath $partialPath -Destination $backupPath
  Write-Output "Backup berhasil dibuat: $backupPath"
  Write-Output 'Arsip ini tidak dienkripsi oleh script. Simpan dan salin hanya melalui media yang terenkripsi dan aksesnya dibatasi.'
} finally {
  if (Test-Path -LiteralPath $partialPath) {
    Remove-Item -LiteralPath $partialPath -Force
  }
  if ($createdRemoteDump) {
    & docker compose -f $composeFile exec -T db rm -f $remotePath | Out-Null
    if ($LASTEXITCODE -ne 0) {
      Write-Warning 'Arsip sementara tidak dapat dihapus dari container database.'
    }
  }
}
