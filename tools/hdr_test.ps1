# The header of a conversation beside a head as a handle of the head: opens a conversation,
# takes hold of the empty part of its header with the real mouse, carries it and lets go.
# Says where the head and the conversation were and where they are: both must have moved
# by the same amount. The real pointer is moved for about two seconds and put back.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\hdr_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test [-Mode cols|one] [-Dx -300] [-Dy 120]
param([string]$Exe, [string]$DataDir, [string]$Mode = "cols", [int]$Dx = -300, [int]$Dy = 120)
$ErrorActionPreference = "Stop"
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class HT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern bool GetCursorPos(out POINT p);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, UIntPtr e);
  public struct RECT { public int L, T, R, B; }
  public struct POINT { public int X, Y; }
}
"@
[HT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"' + $Mode + '","groups":[{"id":1,"dir":"v"}],"heads":[{"id":102,"x":1500,"y":300,"group":1},{"id":101,"x":1500,"y":370,"group":1}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [HT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [HT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id -and [HT]::IsWindowVisible($w)) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object HT+RECT; [HT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Places($what) {
    "--- $what"
    foreach ($w in (All "MliteHead" | Sort-Object { (Rect $_).T })) { $q = Rect $w; "  glowka   ({0},{1})" -f $q.L, $q.T }
    $c = if ($Mode -eq "cols" -or $Mode -eq "dock") { "MlitePane" } else { "MliteMain" }
    foreach ($w in (All $c)) { $q = Rect $w; "  rozmowa  ({0},{1}) {2}x{3}" -f $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T) }
  }
  $main = (All "MliteMain")[0]
  if ($Mode -eq "cols" -or $Mode -eq "dock") { [HT]::PostMessage($main, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 1000 }
  $head = (All "MliteHead" | Sort-Object { (Rect $_).T })[0]
  [HT]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [HT]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 1800
  Places "przed"
  $c = if ($Mode -eq "cols" -or $Mode -eq "dock") { "MlitePane" } else { "MliteMain" }
  $win = (All $c)[0]; $q = Rect $win
  $was = New-Object HT+POINT; [HT]::GetCursorPos([ref]$was) | Out-Null
  $x = $q.L + 150; $y = $q.T + 28           # the name in the header: the empty part of it
  [HT]::SetCursorPos($x, $y) | Out-Null; Start-Sleep -Milliseconds 150
  [HT]::mouse_event(2, 0, 0, 0, [UIntPtr]::Zero); Start-Sleep -Milliseconds 120
  foreach ($k in 1..10) { [HT]::SetCursorPos($x + [int]($Dx * $k / 10), $y + [int]($Dy * $k / 10)) | Out-Null; [HT]::mouse_event(1, 0, 0, 0, [UIntPtr]::Zero); Start-Sleep -Milliseconds 60 }
  Start-Sleep -Milliseconds 200
  [HT]::mouse_event(4, 0, 0, 0, [UIntPtr]::Zero); Start-Sleep -Milliseconds 300
  [HT]::SetCursorPos($was.X, $was.Y) | Out-Null
  Start-Sleep -Milliseconds 900
  Places "po przeciagnieciu za naglowek o ($Dx, $Dy)"
  $saved = Get-Content (Join-Path $DataDir "settings.json") -Raw | ConvertFrom-Json
  "  zapisane w ustawieniach: " + (($saved.heads | ForEach-Object { "({0},{1})" -f $_.x, $_.y }) -join " ")
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
