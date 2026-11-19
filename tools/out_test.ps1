# The preview of a film in a conversation beside a head: scrolls the conversation back,
# takes a picture, and optionally clicks a point of the conversation (the corner button
# that opens the film where it lives). Nothing is really opened: the program only writes
# what it would open, and this prints it.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\out_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Wheel 6] [-ClickX 250 -ClickY 120]
param([string]$Exe, [string]$DataDir, [string]$OutDir, [int]$Wheel = 0, [int]$ClickX = -1, [int]$ClickY = -1, [string]$Tag = "podglad-filmu", [switch]$External)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class OT {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[OT]::SetProcessDPIAware() | Out-Null
$ext = if ($External) { "true" } else { "false" }
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"cols","video_external":' + $ext + ',"heads":[{"id":106,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$log = Join-Path $DataDir "linnet.log"
$before = if (Test-Path $log) { (Get-Content $log).Count } else { 0 }
$env:MLITE_DRYOPEN = "1"
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function One($cls) {
    $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [OT]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [OT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id -and [OT]::IsWindowVisible($w)) { return $w }
    }
    return [IntPtr]::Zero
  }
  function Click($h, $x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [OT]::PostMessage($h, 0x0201, [IntPtr]1, $lp) | Out-Null
    [OT]::PostMessage($h, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1200
  }
  Click (One "MliteHead") 20 20
  $pane = One "MlitePane"
  $view = [OT]::FindWindowEx($pane, [IntPtr]::Zero, "MliteView", [NullString]::Value)
  for ($k = 0; $k -lt $Wheel; $k++) { [OT]::PostMessage($view, 0x020A, [IntPtr](120 -shl 16), [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 250 }
  Start-Sleep -Milliseconds 900
  if ($ClickX -ge 0) { Click $view $ClickX $ClickY }
  $q = New-Object OT+RECT; [OT]::GetWindowRect($pane, [ref]$q) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
  $g.Dispose(); $bmp.Save((Join-Path $OutDir "$Tag.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  "odtwarzacz w oknie rozmowy: {0}" -f ([OT]::FindWindowEx($pane, [IntPtr]::Zero, "MliteViewer", [NullString]::Value) -ne [IntPtr]::Zero)
  if (Test-Path $log) { Get-Content $log | Select-Object -Skip $before | Select-String "would open" | ForEach-Object { "otworzylby: " + ($_ -replace '.*"file":"([^"]*)".*', '$1') } }
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  $env:MLITE_DRYOPEN = $null
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
