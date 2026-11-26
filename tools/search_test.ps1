# The search of a conversation with many finds: opens a long conversation in the ordinary
# window, searches it, scrolls what was found and turns a page. The field has to stay in its
# place while what was found scrolls under it.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\search_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots [-Mode cols|dock|one|tabs] [-Theme light|dark] [-Back FFFFFF]
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
  function Shot($name) {
    $q = Rect $main
    $bmp = New-Object System.Drawing.Bitmap ($q.R - $q.L), ($q.B - $q.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp); $g.CopyFromScreen($q.L, $q.T, 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "zrzut $name.png"
  }
  function Click($w, $x, $y) {
    $at = [IntPtr](($y -shl 16) -bor $x)
    [ET]::PostMessage($w, 0x0201, [IntPtr]1, $at) | Out-Null; [ET]::PostMessage($w, 0x0202, [IntPtr]::Zero, $at) | Out-Null
  }
  [ET]::SetWindowPos($main, [IntPtr](-1), 100, 60, 1400, 900, 0x0010) | Out-Null
  Start-Sleep -Milliseconds 600
  $list = [ET]::FindWindowEx($main, [IntPtr]::Zero, "MliteList", [NullString]::Value)
  Click $list 120 488                                   # the long conversation
  Start-Sleep -Milliseconds 1200
  $find = [ET]::FindWindowEx($main, [IntPtr]::Zero, "Button", "Szukaj w rozmowie")
  [ET]::PostMessage($main, 0x0111, [IntPtr]333, $find) | Out-Null
  Start-Sleep -Milliseconds 1000
  $panel = [ET]::FindWindowEx($main, [IntPtr]::Zero, "MlitePanel", [NullString]::Value)
  $edit = [ET]::FindWindowEx($panel, [IntPtr]::Zero, "Edit", [NullString]::Value)
  foreach ($ch in "wiadom".ToCharArray()) { [ET]::PostMessage($edit, 0x0102, [IntPtr][int]$ch, [IntPtr]::Zero) | Out-Null }
  Start-Sleep -Milliseconds 900; Shot "szukaj-1-wyniki"
  for ($i = 0; $i -lt 2; $i++) { [ET]::PostMessage($panel, 0x020A, [IntPtr](-120 * 65536 * 3), [IntPtr]::Zero) | Out-Null }
  Start-Sleep -Milliseconds 900; Shot "szukaj-2-przewiniete"
  for ($i = 0; $i -lt 8; $i++) { [ET]::PostMessage($panel, 0x020A, [IntPtr](-120 * 65536 * 3), [IntPtr]::Zero) | Out-Null }
  Start-Sleep -Milliseconds 900; Shot "szukaj-3-dol"
  "panel: {0}, pole: {1}" -f ($panel -ne [IntPtr]::Zero), ($edit -ne [IntPtr]::Zero)
  $p.Refresh(); "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if ($sheet) { $sheet.Close(); $sheet.Dispose() }
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
