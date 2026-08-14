param(
  [Parameter(Mandatory = $true)][string]$Server,
  [Parameter(Mandatory = $true)][string]$User,
  [Parameter(Mandatory = $true)][string]$Workspace,
  [string]$Name = $env:COMPUTERNAME,
  [string]$Codex = "@bundled"
)
$ErrorActionPreference = "Stop"
$base = if ($env:DEEIX_AGENT_RELEASE_BASE) { $env:DEEIX_AGENT_RELEASE_BASE } else { "$($Server.TrimEnd('/'))/agent/releases/current" }
$archive = "deeix-agent-bridge-windows-x64.zip"
$installDir = if ($env:DEEIX_AGENT_HOME) { $env:DEEIX_AGENT_HOME } else { Join-Path $env:LOCALAPPDATA "DEEIX\AgentBridge" }
$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("deeix-agent-" + [guid]::NewGuid().ToString("N"))
$suffix = [guid]::NewGuid().ToString("N")
$staged = "$installDir.new.$suffix"
$backup = "$installDir.old.$suffix"
New-Item -ItemType Directory -Force -Path $temporary, $staged | Out-Null
try {
  Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $temporary $archive)
  Invoke-WebRequest "$base/$archive.sha256" -OutFile (Join-Path $temporary "$archive.sha256")
  $expected = ((Get-Content (Join-Path $temporary "$archive.sha256") -Raw).Trim() -split "\s+")[0]
  $actual = (Get-FileHash (Join-Path $temporary $archive) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actual -ne $expected.ToLowerInvariant()) { throw "Agent Bridge checksum mismatch" }
  Expand-Archive (Join-Path $temporary $archive) -DestinationPath $staged -Force
  $stagedBridge = Join-Path $staged "deeix-agent-bridge.cmd"
  & $stagedBridge install --server $Server --user $User --workspace $Workspace --name $Name --codex $Codex
  Stop-ScheduledTask -TaskName "DEEIX Agent Bridge" -ErrorAction SilentlyContinue
  Start-Sleep -Milliseconds 500
  if (Test-Path -LiteralPath $installDir) { Move-Item -LiteralPath $installDir -Destination $backup }
  try {
    Move-Item -LiteralPath $staged -Destination $installDir
  } catch {
    if (Test-Path -LiteralPath $backup) { Move-Item -LiteralPath $backup -Destination $installDir }
    throw
  }
  try {
    $bridge = Join-Path $installDir "deeix-agent-bridge.cmd"
    $action = New-ScheduledTaskAction -Execute "cmd.exe" -Argument "/c `"`"$bridge`" start`""
    $trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
    $settings = New-ScheduledTaskSettingsSet -RestartCount 20 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
    Register-ScheduledTask -TaskName "DEEIX Agent Bridge" -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
    Stop-ScheduledTask -TaskName "DEEIX Agent Bridge" -ErrorAction SilentlyContinue
    Start-ScheduledTask -TaskName "DEEIX Agent Bridge"
    Remove-Item -LiteralPath $backup -Recurse -Force -ErrorAction SilentlyContinue
  } catch {
    Stop-ScheduledTask -TaskName "DEEIX Agent Bridge" -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $installDir -Recurse -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $backup) {
      Move-Item -LiteralPath $backup -Destination $installDir
      Start-ScheduledTask -TaskName "DEEIX Agent Bridge" -ErrorAction SilentlyContinue
    }
    throw
  }
  Write-Host "DEEIX Agent Bridge is installed and running."
} finally {
  Remove-Item -LiteralPath $temporary, $staged -Recurse -Force -ErrorAction SilentlyContinue
}
