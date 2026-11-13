# Heads tied into groups: a stack of two on the right and a head on its own on the left, each
# with its own row of conversations. Takes pictures of the rows, of a row after its size was
# changed, of the ordinary window reached from a conversation, and of a folded group.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\groups_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots -Mode cols
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [int]$HeadSize = 64, [switch]$Fold, [string]$Tag = "")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class GT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int ht, bool paint);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[GT]::SetProcessDPIAware() | Out-Null
$f = if ($Fold) { "true" } else { "false" }
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"' + $Mode + '","head_size":' + $HeadSize +
  ',"groups":[{"id":1,"dir":"v","fold":' + $f + '}],"heads":[{"id":102,"x":1500,"y":260,"group":1},{"id":101,"x":1500,"y":340,"group":1},{"id":106,"x":120,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$name = "grupy-$Mode$Tag"
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [GT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [GT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id -and [GT]::IsWindowVisible($w)) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object GT+RECT; [GT]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  function Click($h, $x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [GT]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [GT]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1500
  }
  function Frame($file) {
    $wins = @(All "MliteMain") + @(All "MlitePane") + @(All "MliteHead")
    $l = 100000; $t = 100000; $r = -100000; $b = -100000
    foreach ($w in $wins) { $q = Rect $w
      if ($q.L -lt $l) { $l = $q.L }; if ($q.T -lt $t) { $t = $q.T }; if ($q.R -gt $r) { $r = $q.R }; if ($q.B -gt $b) { $b = $q.B } }
    $pad = 12
    $bmp = New-Object System.Drawing.Bitmap (($r - $l) + 2*$pad), (($b - $t) + 2*$pad)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen(($l - $pad), ($t - $pad), 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$file.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "  {0}:" -f $file
    foreach ($c in "MliteHead", "MlitePane", "MliteMain") { foreach ($w in @(All $c)) { $q = Rect $w; "    {0} ({1},{2}) {3}x{4}" -f $c.Substring(5), $q.L, $q.T, ($q.R - $q.L), ($q.B - $q.T) } }
  }
  function Heads { All "MliteHead" | Sort-Object { $q = Rect $_; $q.L * 10000 + $q.T } }
  Frame "$name-glowki"
  $hs = @(Heads)
  "widocznych glowek: {0}" -f $hs.Count
  if ($Fold) {
    Click $hs[$hs.Count - 1] 20 20; Frame "$name-rozwinieta"   # the one head of the folded group opens it out
    $hs = @(Heads)
  }
  foreach ($h in $hs) { Click $h 20 20 }
  Frame "$name-otwarte"
  $panes = @(All "MlitePane" | Sort-Object { (Rect $_).L })
  "okien rozmow: {0}" -f $panes.Count
  if ($panes.Count -ge 2) {
    # the outermost conversation of the right group is pulled wider and taller, then let go
    $far = $panes[1]; $q = Rect $far
    [GT]::MoveWindow($far, $q.L - 80, $q.T, ($q.R - $q.L) + 80, ($q.B - $q.T) - 60, $true) | Out-Null
    [GT]::PostMessage($far, 0x0232, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Start-Sleep -Milliseconds 1200
    Frame "$name-rozmiar"
    $q = Rect $far
    $off = if ($Mode -eq "dock") { 108 } else { 68 }   # left of the cross, and of the fold button in the dock
    Click $far (($q.R - $q.L) - $off) 28   # the button to the ordinary window of the program
    Start-Sleep -Milliseconds 800
    Frame "$name-okno-programu"
    "okno programu widoczne: {0}, okien rozmow: {1}" -f (@(All "MliteMain").Count -gt 0), @(All "MlitePane").Count
  } elseif (@(All "MliteMain").Count -gt 0) {
    $m = (All "MliteMain")[0]; $q = Rect $m
    $btn = [GT]::FindWindowEx($m, [IntPtr]::Zero, "Button", "Otwórz w oknie programu")   # the same button in the panel
    "przycisk w panelu: {0}, widoczny: {1}" -f ($btn -ne [IntPtr]::Zero), [GT]::IsWindowVisible($btn)
    [GT]::PostMessage($btn, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Start-Sleep -Milliseconds 800
    Frame "$name-okno-programu"
    $q = Rect ((All "MliteMain")[0]); "okno programu: {0}x{1}" -f ($q.R - $q.L), ($q.B - $q.T)
  }
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
