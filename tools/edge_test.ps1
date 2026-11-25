# The edge of a conversation: opens a conversation from a head, lays a plain sheet of one
# colour under it and takes a picture with room around, so that it can be seen where the
# window ends on that colour.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\edge_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Mode cols|dock|one|tabs] [-Theme light|dark] [-Back FFFFFF]
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "cols", [string]$Theme = "light", [string]$Back = "FFFFFF", [long]$Open = 102, [long]$Other = 101)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class ET {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int w, int hh, uint f);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[ET]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"theme":"' + $Theme + '","head_mode":"' + $Mode + '","heads":[{"id":' + $Open + ',"x":1500,"y":300},{"id":' + $Other + ',"x":1500,"y":380}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
$sheet = $null
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [ET]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [ET]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  function Rect($h) { $r = New-Object ET+RECT; [ET]::GetWindowRect($h, [ref]$r) | Out-Null; return $r }
  $main = (All "MliteMain")[0]
  $multi = ($Mode -eq "cols" -or $Mode -eq "dock")
  if ($multi) { [ET]::PostMessage($main, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null; Start-Sleep -Milliseconds 1000 }
  $head = (All "MliteHead" | Sort-Object { (Rect $_).T })[0]
  [ET]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [ET]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 1800
  $l = 100000; $t = 100000; $r = -100000; $b = -100000
  foreach ($c in @("MliteHead", "MlitePane", "MliteMain")) {
    foreach ($w in (All $c)) {
      if (-not [ET]::IsWindowVisible($w)) { continue }
      $q = Rect $w
      if ($q.L -lt $l) { $l = $q.L }; if ($q.T -lt $t) { $t = $q.T }
      if ($q.R -gt $r) { $r = $q.R }; if ($q.B -gt $b) { $b = $q.B }
    }
  }
  $m = 70
  $l -= $m; $t -= $m; $r += $m; $b += $m
  if ($l -lt 0) { $l = 0 }; if ($t -lt 0) { $t = 0 }
  $sheet = New-Object System.Windows.Forms.Form
  $sheet.FormBorderStyle = "None"; $sheet.ShowInTaskbar = $false; $sheet.StartPosition = "Manual"
  $sheet.BackColor = [System.Drawing.ColorTranslator]::FromHtml("#" + $Back)
  $sheet.Bounds = New-Object System.Drawing.Rectangle $l, $t, ($r - $l), ($b - $t)
  $sheet.TopMost = $true
  [ET]::ShowWindow($sheet.Handle, 4) | Out-Null     # shown without taking the conversation's place as the active window
  foreach ($c in @("MliteMain", "MlitePane", "MliteHead")) {   # the program's windows back over the sheet
    foreach ($w in (All $c)) { if ([ET]::IsWindowVisible($w)) { [ET]::SetWindowPos($w, [IntPtr](-1), 0, 0, 0, 0, 0x0013) | Out-Null } }
  }
  [System.Windows.Forms.Application]::DoEvents()
  Start-Sleep -Milliseconds 700
  [System.Windows.Forms.Application]::DoEvents()
  $bmp = New-Object System.Drawing.Bitmap ($r - $l), ($b - $t)
  $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($l, $t, 0, 0, $bmp.Size)
  $g.Dispose()
  $name = "krawedz-$Mode-$Theme-$Back.png"
  $bmp.Save((Join-Path $OutDir $name), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
  "zrzut $name"
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if ($sheet) { $sheet.Close(); $sheet.Dispose() }
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
