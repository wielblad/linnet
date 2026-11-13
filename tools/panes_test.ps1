# Opens three conversations from heads with "Kolumny" or "Dok" and takes pictures of the
# panel and the panes beside it: all three open, one written in, one folded or closed, and
# the panel put away.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\panes_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots -Mode cols
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [string]$Theme = "")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class PT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[PT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"theme":"' + $Theme + '","head_mode":"' + $Mode + '","heads":[{"id":102,"x":1500,"y":260},{"id":101,"x":1500,"y":340},{"id":106,"x":1500,"y":420}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [PT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [PT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object PT+RECT; [PT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  $main = (All "MliteMain")[0]
  $heads = All "MliteHead" | Sort-Object { (Rect $_).T }
  "glowek: {0}" -f $heads.Count
  function Click($h, $x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [PT]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [PT]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1500
  }
  function Frame($name) {
    $wins = @()
    if ([PT]::IsWindowVisible($main)) { $wins += $main }
    $wins += @(All "MlitePane" | Where-Object { [PT]::IsWindowVisible($_) })
    $l = 100000; $t = 100000; $r = -100000; $b = -100000
    foreach ($w in ($wins + $heads)) {
      $q = Rect $w
      if ($q.L -lt $l) { $l = $q.L }; if ($q.T -lt $t) { $t = $q.T }
      if ($q.R -gt $r) { $r = $q.R }; if ($q.B -gt $b) { $b = $q.B }
    }
    $pad = 12
    $bmp = New-Object System.Drawing.Bitmap (($r - $l) + 2*$pad), (($b - $t) + 2*$pad)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen(($l - $pad), ($t - $pad), 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "  {0}: okien {1}" -f $name, $wins.Count
    foreach ($w in $wins) { $q = Rect $w; "    ({0},{1}) {2}x{3}" -f $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T) }
  }
  Click $heads[0] 20 20; Frame "$Mode-1"
  Click $heads[1] 20 20; Click $heads[2] 20 20; Frame "$Mode-3"
  $panes = @(All "MlitePane" | Sort-Object { (Rect $_).L })
  "paneli: {0}" -f $panes.Count
  if ($panes.Count -gt 0) {
    $pane = $panes[0]
    $edit = [PT]::FindWindowEx($pane, [IntPtr]::Zero, "Edit", [NullString]::Value)
    foreach ($ch in "Test z drugiej rozmowy".ToCharArray()) { [PT]::PostMessage($edit, 0x0102, [IntPtr][int]$ch, [IntPtr]::Zero) | Out-Null }
    Start-Sleep -Milliseconds 400
    [PT]::PostMessage($edit, 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null   # Enter
    Start-Sleep -Milliseconds 1500
    Frame "$Mode-wyslane"
    $q = Rect $pane
    if ($Mode -eq "dock") {   # fold the pane with its "–" button
      Click $pane (($q.R - $q.L) - 68) 26; Frame "$Mode-zwiniety"
    }
  }
  Click $heads[0] 20 20; Frame "$Mode-bez-panelu"   # the head of the panel puts the panel away
  Click $heads[2] 20 20; Frame "$Mode-zamknieta"    # the head of a pane closes its pane
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
