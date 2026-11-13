# Drags a head across the desktop with its panel open and takes pictures of the screen while
# it travels, to see whether the panel is torn or leaves stripes behind.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\overlay_drag.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots
param([string]$Exe, [string]$DataDir, [string]$OutDir)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class DG {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[DG]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"heads":[{"id":102,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function Find($cls) {
    $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [DG]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
      $wpid = 0; [DG]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { return $w }
    }
    return [IntPtr]::Zero
  }
  $head = Find "MliteHead"
  $main = Find "MliteMain"
  if ($head -eq [IntPtr]::Zero) { "brak glowki"; return }
  # a click on the head opens the panel
  [DG]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [DG]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Seconds 3
  "panel widoczny: {0}" -f [DG]::IsWindowVisible($main)
  function Frame($name) {
    $hr = New-Object DG+RECT; [DG]::GetWindowRect($head, [ref]$hr) | Out-Null
    $mr = New-Object DG+RECT; [DG]::GetWindowRect($main, [ref]$mr) | Out-Null
    $l = [Math]::Min($hr.L, $mr.L) - 16; $t = [Math]::Min($hr.T, $mr.T) - 16
    $r = [Math]::Max($hr.R, $mr.R) + 16; $b = [Math]::Max($hr.B, $mr.B) + 16
    $bmp = New-Object System.Drawing.Bitmap ($r - $l), ($b - $t)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($l, $t, 0, 0, $bmp.Size)
    $g.Dispose()
    $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    "  {0}: glowka ({1},{2}) panel ({3},{4})" -f $name, $hr.L, $hr.T, $mr.L, $mr.T
  }
  Frame "drag-0-start"
  $hr = New-Object DG+RECT; [DG]::GetWindowRect($head, [ref]$hr) | Out-Null
  [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point (($hr.L + 20), ($hr.T + 20))
  [DG]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  $shot = 1
  foreach ($step in 40, 90, 150, 220, 300, 380, 460, 540) {
    [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point (($hr.L + 20 - $step), ($hr.T + 20 + [int]($step / 3)))
    [DG]::PostMessage($head, 0x0200, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
    Start-Sleep -Milliseconds 90
    if ($step -eq 150 -or $step -eq 380) { Frame ("drag-{0}-w-ruchu" -f $shot); $shot++ }
  }
  [DG]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 700
  Frame "drag-3-koniec"
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
