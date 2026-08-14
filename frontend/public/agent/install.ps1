param(
  [Parameter(Mandatory = $true)][string]$Server,
  [Parameter(Mandatory = $true)][string]$User,
  [Parameter(Mandatory = $true)][string]$Workspace,
  [string]$Name = $env:COMPUTERNAME,
  [string]$Codex = "codex"
)
$ErrorActionPreference = "Stop"
$base = if ($env:DEEIX_AGENT_RELEASE_BASE) { $env:DEEIX_AGENT_RELEASE_BASE } else { "$($Server.TrimEnd('/'))/agent/releases/current" }
$asset = "deeix-agent-windows-x64.exe"
$installDir = if ($env:DEEIX_AGENT_HOME) { $env:DEEIX_AGENT_HOME } else { Join-Path $env:LOCALAPPDATA "Programs\DEEIX Agent" }
$dataDir = if ($env:DEEIX_AGENT_DATA_DIR) { $env:DEEIX_AGENT_DATA_DIR } else { Join-Path $env:LOCALAPPDATA "DEEIX\Agent" }
$installed = Join-Path $installDir "deeix-agent.exe"
$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("deeix-agent-" + [guid]::NewGuid().ToString("N"))
$download = Join-Path $temporary $asset
$backup = "$installed.previous"
$taskName = "DEEIX Agent"
$userSID = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$serviceInstalled = $false
$hadScheduledTask = $null -ne (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)
New-Item -ItemType Directory -Force -Path $temporary, $installDir, $dataDir | Out-Null
try {
  Invoke-WebRequest "$base/$asset" -OutFile $download
  Invoke-WebRequest "$base/$asset.sha256" -OutFile "$download.sha256"
  $expected = ((Get-Content "$download.sha256" -Raw).Trim() -split "\s+")[0]
  $actual = (Get-FileHash $download -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) { throw "DEEIX Agent checksum mismatch" }

  Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 700
  & $download install --server $Server --user $User --workspace $Workspace --name $Name --codex $Codex --data-dir $dataDir
  if ($LASTEXITCODE -ne 0) { throw "DEEIX Agent configuration failed" }

  $installedLiteral = $installed.Replace("'", "''")
  $downloadLiteral = $download.Replace("'", "''")
  $backupLiteral = $backup.Replace("'", "''")
  $dataDirLiteral = $dataDir.Replace("'", "''")
  $userSIDLiteral = $userSID.Replace("'", "''")
  $errorFile = Join-Path $temporary "service-install.error.txt"
  $errorFileLiteral = $errorFile.Replace("'", "''")
  $serviceScript = @"
`$ErrorActionPreference = 'Stop'
try {
  & '$downloadLiteral' service-stop
  if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent service stop failed' }
  Remove-Item -LiteralPath '$backupLiteral' -Force -ErrorAction SilentlyContinue
  if (Test-Path -LiteralPath '$installedLiteral') { Move-Item -LiteralPath '$installedLiteral' -Destination '$backupLiteral' }
  try {
    Move-Item -LiteralPath '$downloadLiteral' -Destination '$installedLiteral'
    & '$installedLiteral' service-install --data-dir '$dataDirLiteral' --user-sid '$userSIDLiteral'
    if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent service installation failed' }
  } catch {
    if (Test-Path -LiteralPath '$installedLiteral') { & '$installedLiteral' service-uninstall 2>`$null }
    Remove-Item -LiteralPath '$installedLiteral' -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath '$backupLiteral') { Move-Item -LiteralPath '$backupLiteral' -Destination '$installedLiteral' }
    throw
  }
} catch {
  [IO.File]::WriteAllText('$errorFileLiteral', `$_.Exception.Message)
  exit 1
}
"@
  Remove-Item -LiteralPath (Join-Path $dataDir "runtime-status.json") -Force -ErrorAction SilentlyContinue
  $encodedServiceScript = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($serviceScript))
  $elevated = Start-Process powershell.exe -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedServiceScript"
  if ($elevated.ExitCode -ne 0) {
    $detail = if (Test-Path -LiteralPath $errorFile) { (Get-Content -LiteralPath $errorFile -Raw).Trim() } else { "administrator approval was not completed" }
    throw "DEEIX Agent system service installation failed: $detail"
  }
  $serviceInstalled = $true

  $connected = $false
  for ($attempt = 0; $attempt -lt 90; $attempt++) {
    Start-Sleep -Seconds 1
    $statusPath = Join-Path $dataDir "runtime-status.json"
    if (-not (Test-Path -LiteralPath $statusPath)) { continue }
    try {
      $status = Get-Content -LiteralPath $statusPath -Raw | ConvertFrom-Json
      if ($status.state -eq "connected") { $connected = $true; break }
    } catch {}
  }
  if (-not $connected) {
    $logPath = Join-Path $dataDir "agent.log"
    $detail = if (Test-Path -LiteralPath $logPath) { (Get-Content -LiteralPath $logPath -Tail 1) } else { "no runtime log was written" }
    throw "DEEIX Agent service did not connect: $detail"
  }
  Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
  Write-Host "DEEIX Agent system service is installed and connected: $installed"
} catch {
  if (-not $serviceInstalled -and $hadScheduledTask) { Start-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue }
  throw
} finally {
  Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
