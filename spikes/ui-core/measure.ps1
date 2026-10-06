# Sample CPU and memory of the given processes (and their children by name) for N seconds.
# Usage: powershell -File measure.ps1 -Names app1,app2 -Seconds 20
param([string[]]$Names, [int]$Seconds = 20)
$cores = [Environment]::ProcessorCount
function Snap { Get-Process -Name $Names -ErrorAction SilentlyContinue }
$prev = @{}; foreach ($p in Snap) { $prev[$p.Id] = $p.TotalProcessorTime.TotalMilliseconds }
$t0 = Get-Date; $peak = 0; $cpuSamples = @()
for ($i = 0; $i -lt $Seconds; $i++) {
  Start-Sleep -Seconds 1
  $now = Snap; $used = 0; $ws = 0
  foreach ($p in $now) {
    $ms = $p.TotalProcessorTime.TotalMilliseconds
    if ($prev.ContainsKey($p.Id)) { $used += $ms - $prev[$p.Id] }
    $prev[$p.Id] = $ms; $ws += $p.WorkingSet64
  }
  $cpuSamples += $used / 10 / $cores
  if ($ws -gt $peak) { $peak = $ws }
}
$avg = ($cpuSamples | Measure-Object -Average).Average
"processes: " + ((Snap | ForEach-Object { $_.ProcessName + "(" + $_.Id + ")" }) -join ", ")
"avg_cpu_percent_of_machine: {0:N1}  (cores={1})" -f $avg, $cores
"peak_working_set_mb: {0:N0}" -f ($peak / 1MB)
