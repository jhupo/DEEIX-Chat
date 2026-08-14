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
New-Item -ItemType Directory -Force -Path $temporary, $installDir, $dataDir | Out-Null
try {
  Invoke-WebRequest "$base/$asset" -OutFile $download
  Invoke-WebRequest "$base/$asset.sha256" -OutFile "$download.sha256"
  $expected = ((Get-Content "$download.sha256" -Raw).Trim() -split "\s+")[0]
  $actual = (Get-FileHash $download -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) { throw "DEEIX Agent checksum mismatch" }

  & $download install --server $Server --user $User --workspace $Workspace --name $Name --codex $Codex --data-dir $dataDir
  if ($LASTEXITCODE -ne 0) { throw "DEEIX Agent configuration failed" }

  Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 700
  Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
  if (Test-Path -LiteralPath $installed) { Move-Item -LiteralPath $installed -Destination $backup }
  try {
    Move-Item -LiteralPath $download -Destination $installed
  } catch {
    if (Test-Path -LiteralPath $backup) { Move-Item -LiteralPath $backup -Destination $installed }
    throw
  }

  try {
    $action = New-ScheduledTaskAction -Execute $installed -Argument "start --data-dir `"$dataDir`""
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $settings = New-ScheduledTaskSettingsSet -RestartCount 20 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
    Remove-Item -LiteralPath (Join-Path $dataDir "runtime-status.json") -Force -ErrorAction SilentlyContinue
    Start-ScheduledTask -TaskName $taskName
    $started = $false
    for ($attempt = 0; $attempt -lt 15; $attempt++) {
      Start-Sleep -Seconds 1
      if (Test-Path -LiteralPath (Join-Path $dataDir "runtime-status.json")) { $started = $true; break }
    }
    if (-not $started) { throw "DEEIX Agent did not start; run deeix-agent doctor for details" }
    Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
  } catch {
    Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $installed -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $backup) {
      Move-Item -LiteralPath $backup -Destination $installed
      Start-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
    }
    throw
  }
  Write-Host "DEEIX Agent is installed and running: $installed"
} finally {
  Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
