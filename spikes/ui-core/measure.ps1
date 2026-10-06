# Sample CPU and memory of a process tree (root + all descendants, e.g. WebView2 children) for N seconds.
# Usage: powershell -File measure.ps1 -RootPid 1234 -Seconds 20
# Also prints the root window title (UI spikes write the displayed fps into the title).
param([int]$RootPid, [int]$Seconds = 20)
$cores = [Environment]::ProcessorCount
function Tree {
  $all = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId
  $ids = @($RootPid); $added = $true
  while ($added) {
    $added = $false
    foreach ($p in $all) {
      if (($ids -contains $p.ParentProcessId) -and -not ($ids -contains $p.ProcessId)) { $ids += $p.ProcessId; $added = $true }
    }
  }
  Get-Process -Id $ids -ErrorAction SilentlyContinue
}
$prev = @{}; foreach ($p in Tree) { $prev[$p.Id] = $p.TotalProcessorTime.TotalMilliseconds }
$peak = 0; $cpuSamples = @()
for ($i = 0; $i -lt $Seconds; $i++) {
  Start-Sleep -Seconds 1
  $used = 0; $ws = 0
  foreach ($p in Tree) {
    $ms = $p.TotalProcessorTime.TotalMilliseconds
    if ($prev.ContainsKey($p.Id)) { $used += $ms - $prev[$p.Id] }
    $prev[$p.Id] = $ms; $ws += $p.WorkingSet64
  }
  $cpuSamples += $used / 10 / $cores
  if ($ws -gt $peak) { $peak = $ws }
}
$avg = ($cpuSamples | Measure-Object -Average).Average
"processes: " + ((Tree | ForEach-Object { $_.ProcessName + "(" + $_.Id + ")" }) -join ", ")
"avg_cpu_percent_of_machine: {0:N1}  (cores={1})" -f $avg, $cores
"peak_working_set_mb: {0:N0}" -f ($peak / 1MB)
"window_title: " + (Get-Process -Id $RootPid -ErrorAction SilentlyContinue).MainWindowTitle
