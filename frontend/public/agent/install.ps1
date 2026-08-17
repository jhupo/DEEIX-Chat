param(
  [Parameter(Mandatory = $true)][string]$Server,
  [Parameter(Mandatory = $true)][string]$User,
  [string]$Workspace,
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
$legacyTaskName = "DEEIX Agent Bridge"
$legacyInstallDir = Join-Path $env:LOCALAPPDATA "DEEIX\AgentBridge"
$userSID = if ($env:DEEIX_AGENT_USER_SID) { $env:DEEIX_AGENT_USER_SID } else { [Security.Principal.WindowsIdentity]::GetCurrent().User.Value }
$serviceInstalled = $false
$hadScheduledTask = $null -ne (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue)
$hadLegacyScheduledTask = $null -ne (Get-ScheduledTask -TaskName $legacyTaskName -ErrorAction SilentlyContinue)
$hadService = $null -ne (Get-Service -Name "DEEIXAgent" -ErrorAction SilentlyContinue)
New-Item -ItemType Directory -Force -Path $temporary, $installDir, $dataDir | Out-Null
try {
  Write-Host "DEEIX Agent: downloading and verifying the current release..."
  Invoke-WebRequest "$base/$asset" -OutFile $download
  Invoke-WebRequest "$base/$asset.sha256" -OutFile "$download.sha256"
  $expected = ((Get-Content "$download.sha256" -Raw).Trim() -split "\s+")[0]
  $actual = (Get-FileHash $download -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) { throw "DEEIX Agent checksum mismatch" }
  $downloadVersion = ((& $download version 2>&1) | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or -not $downloadVersion) { throw "DEEIX Agent version check failed" }

  @($taskName, $legacyTaskName) | ForEach-Object {
    Stop-ScheduledTask -TaskName $_ -ErrorAction SilentlyContinue
  }
  if ($hadScheduledTask -or $hadLegacyScheduledTask) {
    $taskStopped = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
      $runningTasks = @(
        @($taskName, $legacyTaskName) | ForEach-Object {
          Get-ScheduledTask -TaskName $_ -ErrorAction SilentlyContinue
        } | Where-Object { $_.State -eq "Running" }
      )
      if ($runningTasks.Count -eq 0) { $taskStopped = $true; break }
      Start-Sleep -Seconds 1
    }
    if (-not $taskStopped) { throw "DEEIX Agent scheduled tasks did not stop" }
  }
  Write-Host "DEEIX Agent: configuring this device..."
  $installArgs = @("install", "--server", $Server, "--user", $User, "--name", $Name, "--codex", $Codex, "--data-dir", $dataDir)
  if ($Workspace) { $installArgs += @("--workspace", $Workspace) }
  & $download @installArgs
  if ($LASTEXITCODE -ne 0) { throw "DEEIX Agent configuration failed" }

  $installedLiteral = $installed.Replace("'", "''")
  $downloadLiteral = $download.Replace("'", "''")
  $backupLiteral = $backup.Replace("'", "''")
  $dataDirLiteral = $dataDir.Replace("'", "''")
  $legacyInstallDirLiteral = $legacyInstallDir.Replace("'", "''")
  $userSIDLiteral = $userSID.Replace("'", "''")
  $errorFile = Join-Path $temporary "service-install.error.txt"
  $errorFileLiteral = $errorFile.Replace("'", "''")
  $serviceScript = @"
`$ErrorActionPreference = 'Stop'
try {
  & '$downloadLiteral' service-stop
  if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent service stop failed' }
  `$remaining = @(Get-CimInstance Win32_Process | Where-Object {
    `$commandLine = [string]`$_.CommandLine
    [string]::Equals(`$_.ExecutablePath, '$installedLiteral', [StringComparison]::OrdinalIgnoreCase) -or
      (`$_.Name -in @('deeix-agent.exe', 'node.exe', 'cmd.exe') -and
        (`$commandLine.IndexOf('$dataDirLiteral', [StringComparison]::OrdinalIgnoreCase) -ge 0 -or
         `$commandLine.IndexOf('$legacyInstallDirLiteral', [StringComparison]::OrdinalIgnoreCase) -ge 0))
  })
  foreach (`$process in `$remaining) { Stop-Process -Id `$process.ProcessId -Force -ErrorAction SilentlyContinue }
  for (`$attempt = 0; `$attempt -lt 30; `$attempt++) {
    `$remaining = @(Get-CimInstance Win32_Process | Where-Object {
      `$commandLine = [string]`$_.CommandLine
      [string]::Equals(`$_.ExecutablePath, '$installedLiteral', [StringComparison]::OrdinalIgnoreCase) -or
        (`$_.Name -in @('deeix-agent.exe', 'node.exe', 'cmd.exe') -and
          (`$commandLine.IndexOf('$dataDirLiteral', [StringComparison]::OrdinalIgnoreCase) -ge 0 -or
           `$commandLine.IndexOf('$legacyInstallDirLiteral', [StringComparison]::OrdinalIgnoreCase) -ge 0))
    })
    if (`$remaining.Count -eq 0) { break }
    Start-Sleep -Seconds 1
  }
  if (`$remaining.Count -ne 0) { throw 'DEEIX Agent process did not stop' }
  Remove-Item -LiteralPath '$backupLiteral' -Force -ErrorAction SilentlyContinue
  if (Test-Path -LiteralPath '$installedLiteral') { Move-Item -LiteralPath '$installedLiteral' -Destination '$backupLiteral' }
  try {
    Move-Item -LiteralPath '$downloadLiteral' -Destination '$installedLiteral'
    & '$installedLiteral' service-install --data-dir '$dataDirLiteral' --user-sid '$userSIDLiteral'
    if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent service installation failed' }
  } catch {
    if (Test-Path -LiteralPath '$installedLiteral') { & '$installedLiteral' service-uninstall 2>`$null }
    Remove-Item -LiteralPath '$installedLiteral' -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath '$backupLiteral') {
      Move-Item -LiteralPath '$backupLiteral' -Destination '$installedLiteral'
      if ('$hadService' -eq 'True') {
        & '$installedLiteral' service-install --data-dir '$dataDirLiteral' --user-sid '$userSIDLiteral'
        if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent previous service restoration failed' }
      }
    }
    throw
  }
} catch {
  [IO.File]::WriteAllText('$errorFileLiteral', `$_.Exception.Message)
  exit 1
}
"@
  Remove-Item -LiteralPath (Join-Path $dataDir "runtime-status.json") -Force -ErrorAction SilentlyContinue
  $encodedServiceScript = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($serviceScript))
  $principal = [Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())
  $isElevated = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if ($isElevated) {
    & powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedServiceScript
    $elevatedExitCode = $LASTEXITCODE
  } else {
    Write-Host "DEEIX Agent: requesting administrator approval to replace the system service..."
    $elevated = Start-Process powershell.exe -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedServiceScript"
    $elevatedExitCode = $elevated.ExitCode
  }
  if ($elevatedExitCode -ne 0) {
    $detail = if (Test-Path -LiteralPath $errorFile) { (Get-Content -LiteralPath $errorFile -Raw).Trim() } else { "administrator approval was not completed" }
    throw "DEEIX Agent system service installation failed: $detail"
  }
  $serviceInstalled = $true

  Write-Host "DEEIX Agent: waiting up to 120 seconds for a stable connection..."
  $connected = $false
  $connectedSeconds = 0
  for ($attempt = 0; $attempt -lt 120; $attempt++) {
    Start-Sleep -Seconds 1
    $statusPath = Join-Path $dataDir "runtime-status.json"
    if (-not (Test-Path -LiteralPath $statusPath)) { continue }
    try {
      $status = Get-Content -LiteralPath $statusPath -Raw | ConvertFrom-Json
      if ($status.state -eq "connected") {
        $connectedSeconds++
        if ($connectedSeconds -ge 20) { $connected = $true; break }
      } else {
        $connectedSeconds = 0
      }
    } catch {}
    if ($attempt -gt 0 -and $attempt % 10 -eq 0) { Write-Host "DEEIX Agent: connection check $attempt/120..." }
  }
  if (-not $connected) {
    $logPath = Join-Path $dataDir "agent.log"
    $detail = if (Test-Path -LiteralPath $logPath) { (Get-Content -LiteralPath $logPath -Tail 1) } else { "no runtime log was written" }
    throw "DEEIX Agent service did not connect: $detail"
  }
  @($taskName, $legacyTaskName) | ForEach-Object {
    Unregister-ScheduledTask -TaskName $_ -Confirm:$false -ErrorAction SilentlyContinue
  }
  Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
  $installedVersion = ((& $installed version 2>&1) | Out-String).Trim()
  Write-Host "DEEIX Agent system service is installed and connected: $installedVersion ($installed)"
} catch {
  if ($serviceInstalled) {
    $installedLiteral = $installed.Replace("'", "''")
    $backupLiteral = $backup.Replace("'", "''")
    $dataDirLiteral = $dataDir.Replace("'", "''")
    $userSIDLiteral = $userSID.Replace("'", "''")
    $rollbackScript = @"
`$ErrorActionPreference = 'Stop'
if (Test-Path -LiteralPath '$installedLiteral') { & '$installedLiteral' service-uninstall 2>`$null }
Remove-Item -LiteralPath '$installedLiteral' -Force -ErrorAction SilentlyContinue
if (Test-Path -LiteralPath '$backupLiteral') {
  Move-Item -LiteralPath '$backupLiteral' -Destination '$installedLiteral'
  if ('$hadService' -eq 'True') {
    & '$installedLiteral' service-install --data-dir '$dataDirLiteral' --user-sid '$userSIDLiteral'
    if (`$LASTEXITCODE -ne 0) { throw 'DEEIX Agent previous service restoration failed' }
  }
}
"@
    $encodedRollback = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($rollbackScript))
    Start-Process powershell.exe -Verb RunAs -WindowStyle Hidden -Wait -ArgumentList "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedRollback" | Out-Null
  }
  Unregister-ScheduledTask -TaskName $legacyTaskName -Confirm:$false -ErrorAction SilentlyContinue
  if ($hadService) {
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
  } elseif ($hadScheduledTask) {
    Start-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  }
  throw
} finally {
  Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
