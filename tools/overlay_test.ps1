# Opens a conversation from a head on test data and looks at what the panel of the overlay
# is: whether it has a taskbar button, whether it stays on top, and how it looks.
param([string]$Exe, [string]$DataDir, [string]$OutDir)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class OV {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern IntPtr GetWindowLongPtrW(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[OV]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"sound":false,"heads":[{"id":102,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 4
  function Find($cls) {
    $w = [IntPtr]::Zero
    for ($i = 0; $i -lt 300; $i++) {
      $w = [OV]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
      if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
      $wpid = 0; [OV]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
      if ($wpid -eq $p.Id) { return $w }
    }
    return [IntPtr]::Zero
  }
  function Shot($h, $file) {
    $r = New-Object OV+RECT; [OV]::GetWindowRect($h, [ref]$r) | Out-Null
    if ($r.R - $r.L -lt 10) { "zrzut {0}: brak okna" -f $file; return }
    $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $dc = $g.GetHdc(); [OV]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
    $bmp.Save((Join-Path $OutDir $file), [System.Drawing.Imaging.ImageFormat]::Png); $bmp.Dispose()
    "zrzut {0}: {1}x{2}" -f $file, ($r.R - $r.L), ($r.B - $r.T)
  }
  function Styles($h) {
    $ex = [int64][OV]::GetWindowLongPtrW($h, -20)
    $st = [int64][OV]::GetWindowLongPtrW($h, -16)
    "  na pasku zadan: {0} | zawsze na wierzchu: {1} | z ramka okna: {2}" -f (($ex -band 0x80) -eq 0), (($ex -band 0x8) -ne 0), (($st -band 0x00C00000) -ne 0)
  }
  $main = Find "MliteMain"
  $head = Find "MliteHead"
  "przed otwarciem glowki:"; Styles $main
  Shot $head "overlay-glowka.png"
  # a click on the head, as a person would
  [OV]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  [OV]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Seconds 2
  "po otwarciu rozmowy z glowki:"; Styles $main
  Shot $main "overlay-panel.png"
  # a real screen capture, with what Windows draws around the panel
  $mr0 = New-Object OV+RECT; [OV]::GetWindowRect($main, [ref]$mr0) | Out-Null
  $pad = 12
  $bmp2 = New-Object System.Drawing.Bitmap (($mr0.R - $mr0.L) + 2*$pad), (($mr0.B - $mr0.T) + 2*$pad)
  $g2 = [System.Drawing.Graphics]::FromImage($bmp2)
  $g2.CopyFromScreen(($mr0.L - $pad), ($mr0.T - $pad), 0, 0, $bmp2.Size)
  $g2.Dispose()
  $bmp2.Save((Join-Path $OutDir "overlay-ekran.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  $bmp2.Dispose()
  "zrzut z ekranu: overlay-ekran.png"
  # the X closes the panel and leaves the head on the desktop
  $btn = [IntPtr]::Zero
  $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 50; $i++) {
    $w = [OV]::FindWindowEx($main, $w, "Button", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    if ([OV]::GetWindowLongPtrW($w, -12) -eq 335) { $btn = $w }   # GWL_ID
  }
  if ($btn -eq [IntPtr]::Zero) { "nie znalazlem przycisku X" } else {
    [OV]::PostMessage($btn, 0x0201, [IntPtr]1, [IntPtr]0x000A000A) | Out-Null
    [OV]::PostMessage($btn, 0x0202, [IntPtr]::Zero, [IntPtr]0x000A000A) | Out-Null
    Start-Sleep -Seconds 2
    $visible = [OV]::IsWindowVisible($main)
    $headLeft = [OV]::IsWindowVisible($head)
    "po kliknieciu X: panel widoczny {0} | glowka zostala {1}" -f $visible, $headLeft
  }
  # drag the head and see whether the panel travels with it
  $hr = New-Object OV+RECT; [OV]::GetWindowRect($head, [ref]$hr) | Out-Null
  $mr = New-Object OV+RECT; [OV]::GetWindowRect($main, [ref]$mr) | Out-Null
  "przed przesunieciem: glowka ({0},{1}) panel ({2},{3})" -f $hr.L, $hr.T, $mr.L, $mr.T
  [OV]::PostMessage($head, 0x0201, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
  foreach ($step in 60, 160, 260) {
    [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point (($hr.L + 20 - $step), ($hr.T + 20 + 120))
    [OV]::PostMessage($head, 0x0200, [IntPtr]1, [IntPtr]0x00140014) | Out-Null
    Start-Sleep -Milliseconds 300
  }
  [OV]::PostMessage($head, 0x0202, [IntPtr]::Zero, [IntPtr]0x00140014) | Out-Null
  Start-Sleep -Milliseconds 600
  [OV]::GetWindowRect($head, [ref]$hr) | Out-Null
  [OV]::GetWindowRect($main, [ref]$mr) | Out-Null
  "po przesunieciu:    glowka ({0},{1}) panel ({2},{3})" -f $hr.L, $hr.T, $mr.L, $mr.T
  $p.Refresh()
  "program dziala: {0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
