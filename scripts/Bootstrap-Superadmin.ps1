$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $projectRoot 'docker-compose.yml'

$email = Read-Host 'Email superadmin awal'
$securePassword = Read-Host 'Password superadmin (minimal 16 karakter)' -AsSecureString
$confirmPassword = Read-Host 'Ulangi password superadmin' -AsSecureString
$passwordPointer = [IntPtr]::Zero
$confirmPointer = [IntPtr]::Zero
try {
  $passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
  $confirmPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($confirmPassword)
  $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
  $confirmation = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($confirmPointer)
  if ($password -cne $confirmation) { throw 'Password konfirmasi tidak cocok.' }
  if ([System.Text.Encoding]::UTF8.GetByteCount($password) -lt 16 -or [System.Text.Encoding]::UTF8.GetByteCount($password) -gt 72) {
    throw 'Password harus berisi 16 sampai 72 byte UTF-8.'
  }

  $branchCode = Read-Host 'Kode cabang superadmin (BDG atau SBY)'
  $branchCode = $branchCode.Trim().ToUpperInvariant()
  if ($branchCode -notin @('BDG', 'SBY')) { throw 'Kode cabang harus BDG atau SBY.' }

  $env:BOOTSTRAP_SUPERADMIN_EMAIL = $email.Trim()
  $env:BOOTSTRAP_SUPERADMIN_PASSWORD = $password
  $env:BOOTSTRAP_BRANCH_CODE = $branchCode
  docker compose -f $composeFile run --rm -e BOOTSTRAP_SUPERADMIN_EMAIL -e BOOTSTRAP_SUPERADMIN_PASSWORD -e BOOTSTRAP_BRANCH_CODE api bootstrap-superadmin
  if ($LASTEXITCODE -ne 0) { throw 'Bootstrap superadmin gagal. Periksa log container tanpa membagikan password.' }
  Write-Output 'Superadmin awal berhasil dibuat. Login dan ganti password sementara segera.'
} finally {
  Remove-Item Env:BOOTSTRAP_SUPERADMIN_EMAIL -ErrorAction SilentlyContinue
  Remove-Item Env:BOOTSTRAP_SUPERADMIN_PASSWORD -ErrorAction SilentlyContinue
  Remove-Item Env:BOOTSTRAP_BRANCH_CODE -ErrorAction SilentlyContinue
  if ($passwordPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer) }
  if ($confirmPointer -ne [IntPtr]::Zero) { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($confirmPointer) }
  $password = $null
  $confirmation = $null
  $securePassword.Dispose()
  $confirmPassword.Dispose()
}
