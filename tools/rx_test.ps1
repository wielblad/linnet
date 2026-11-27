# Who gave a reaction: opens a conversation in the ordinary window and holds the pointer over
# a point of its messages, the way the mouse does. Without -X it only takes a picture, from
# which the place of the reactions can be read.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\rx_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Row 200] [-X 0 -Y 0] [-Theme light|dark]
param([string]$Exe, [string]$DataDir, [string]$OutDir, [int]$Row = 200, [int]$X = 0, [int]$Y = 0, [string]$Theme = "light", [string]$Name = "reakcje")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class RX {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int w, int hh, uint f);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[RX]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"theme":"' + $Theme + '","head_mode":"","heads":[]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$env:MLITE_KEEPHOVER = "1"
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  $main = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [RX]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; [RX]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { $main = $w; break }
  }
  if ($main -eq [IntPtr]::Zero) { throw "brak okna" }
  function Rect($h) { $r = New-Object RX+RECT; [RX]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Shot($file) {
    $q = Rect $main
    $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$file.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "zrzut $file.png"
  }
  [RX]::SetWindowPos($main, [IntPtr](-1), 100, 60, 1100, 760, 0x0010) | Out-Null
  Start-Sleep -Milliseconds 600
  $list = [RX]::FindWindowEx($main, [IntPtr]::Zero, "MliteList", [NullString]::Value)
  $at = [IntPtr](($Row -shl 16) -bor 120)
  [RX]::PostMessage($list, 0x0201, [IntPtr]1, $at) | Out-Null; [RX]::PostMessage($list, 0x0202, [IntPtr]::Zero, $at) | Out-Null
  Start-Sleep -Milliseconds 1200
  $view = [RX]::FindWindowEx($main, [IntPtr]::Zero, "MliteView", [NullString]::Value)
  $m = Rect $main; $v = Rect $view
  "widok zaczyna sie w oknie: {0},{1}" -f ($v.L - $m.L), ($v.T - $m.T)
  if ($X -gt 0) {   # the point is given as on the picture of the whole window
    $pt = [IntPtr]((($Y - ($v.T - $m.T)) -shl 16) -bor ($X - ($v.L - $m.L)))
    [RX]::PostMessage($view, 0x0200, [IntPtr]::Zero, $pt) | Out-Null
    Start-Sleep -Milliseconds 500
  }
  Shot $Name
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
