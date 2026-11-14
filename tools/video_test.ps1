# Plays one video address in the player of the program and takes pictures of the window a
# few times while the page loads: to see what the player shows for that address.
# The test copy is not signed in to anything.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\video_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots -Url https://...
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Url, [string]$Tag = "film", [int]$Looks = 4)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class VT {
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int w, int ht, uint flags);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[VT]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"heads":[]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$env:MLITE_PLAYVIDEO = $Url
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 3
  $main = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [VT]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; [VT]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { $main = $w; break }
  }
  foreach ($k in 1..$Looks) {
    Start-Sleep -Seconds 4
    [VT]::SetWindowPos($main, [IntPtr](-1), 0, 0, 0, 0, 0x0013) | Out-Null   # above the rest for the picture
    Start-Sleep -Milliseconds 300
    $q = New-Object VT+RECT; [VT]::GetWindowRect($main, [ref]$q) | Out-Null
    $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$Tag-$k.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    [VT]::SetWindowPos($main, [IntPtr](-2), 0, 0, 0, 0, 0x0013) | Out-Null
    "  {0}-{1}.png po {2} s" -f $Tag, $k, (3 + 4 * $k)
  }
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  $env:MLITE_PLAYVIDEO = $null
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Get-Process msedgewebview2 -ErrorAction SilentlyContinue | Where-Object { $_.Path -and $_.StartTime -gt (Get-Date).AddMinutes(-2) -and $_.MainWindowTitle -eq "" } | Out-Null
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
