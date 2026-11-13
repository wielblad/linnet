# Opens several conversations from heads with the "Zakładki" way of showing them and takes
# pictures of the panel: after each head, after a click on a tab and after closing one.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File tools\tabs_test.ps1 -Exe build\linnet.exe -DataDir .local\data-ui-test -OutDir .local\shots
param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Mode = "tabs")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class TB {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[TB]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"head_mode":"' + $Mode + '","heads":[{"id":102,"x":1500,"y":260},{"id":101,"x":1500,"y":340},{"id":106,"x":1500,"y":420}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function All($cls) {
    $out = @(); $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [TB]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { break }
      $wpid = 0; [TB]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { $out += $w }
    }
    return $out
  }
  $main = (All "MliteMain")[0]
  # the heads from top to bottom, whatever order Windows lists them in
  $heads = All "MliteHead" | Sort-Object { $r = New-Object TB+RECT; [TB]::GetWindowRect($_, [ref]$r) | Out-Null; $r.T }
  "glowek: {0}" -f $heads.Count
  function ClickHead($h) {
    [TB]::PostMessage($h, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
    [TB]::PostMessage($h, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
    Start-Sleep -Milliseconds 1500
  }
  function ClickMain($x, $y) {
    $lp = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
    [TB]::PostMessage($main, 0x0201, [IntPtr]1, $lp) | Out-Null
    [TB]::PostMessage($main, 0x0202, [IntPtr]::Zero, $lp) | Out-Null
    Start-Sleep -Milliseconds 1200
  }
  function Frame($name) {
    $mr = New-Object TB+RECT; [TB]::GetWindowRect($main, [ref]$mr) | Out-Null
    $pad = 10
    $bmp = New-Object System.Drawing.Bitmap (($mr.R - $mr.L) + 2*$pad + 90), (($mr.B - $mr.T) + 2*$pad)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen(($mr.L - $pad), ($mr.T - $pad), 0, 0, $bmp.Size)
    $g.Dispose(); $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "  {0}: panel ({1},{2}) {3}x{4}, widoczny {5}" -f $name, $mr.L, $mr.T, ($mr.R - $mr.L), ($mr.B - $mr.T), [TB]::IsWindowVisible($main)
  }
  ClickHead $heads[0]; Frame "tabs-1"
  ClickHead $heads[1]; ClickHead $heads[2]; Frame "tabs-3"
  ClickMain 60 18; Frame "tabs-klik-pierwsza"
  ClickMain 240 18; Frame "tabs-zamknieta-druga"
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
