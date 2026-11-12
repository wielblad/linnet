param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Tag = "dark", [int]$Width = 0, [int]$Height = 0)
# Pasting a picture shows its thumbnail above the text field; a pasted file after it shows text only.
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WD {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern bool GetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern bool SetKeyboardState(byte[] s);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WD]::SetProcessDPIAware() | Out-Null
$keepText = $null; $keepImage = $null; $keepFiles = $null
if ([Windows.Forms.Clipboard]::ContainsText()) { $keepText = [Windows.Forms.Clipboard]::GetText() }
elseif ([Windows.Forms.Clipboard]::ContainsImage()) { $keepImage = [Windows.Forms.Clipboard]::GetImage() }
elseif ([Windows.Forms.Clipboard]::ContainsFileDropList()) { $keepFiles = [Windows.Forms.Clipboard]::GetFileDropList() }
$outbox = Join-Path $DataDir "outbox"
Remove-Item (Join-Path $outbox "*") -ErrorAction SilentlyContinue
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 3
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WD]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; $tid = [WD]::GetWindowThreadProcessId($w, [ref]$wpid)
    if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  if ($Width -gt 0) {
    $r0 = New-Object WD+RECT; [WD]::GetWindowRect($h, [ref]$r0) | Out-Null
    [WD]::MoveWindow($h, $r0.L, $r0.T, $Width, $Height, $true) | Out-Null
    Start-Sleep -Milliseconds 400
  }
  $lb = [WD]::GetDlgItem($h, 311)
  [WD]::SendMessage($lb, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
  [WD]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 1
  if ($Width -gt 0) { [WD]::PostMessage($lb, 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null; Start-Sleep -Milliseconds 800 }
  $edit = [WD]::GetDlgItem($h, 330)
  function CtrlV {
    $me = [WD]::GetCurrentThreadId()
    [WD]::AttachThreadInput($me, $uiThread, $true) | Out-Null
    $st = New-Object byte[] 256
    [WD]::GetKeyboardState($st) | Out-Null
    $st[0x11] = 0x80
    [WD]::SetKeyboardState($st) | Out-Null
    [WD]::PostMessage($edit, 0x0100, [IntPtr]0x56, [IntPtr]1) | Out-Null
    Start-Sleep -Milliseconds 800
    $st[0x11] = 0
    [WD]::SetKeyboardState($st) | Out-Null
    [WD]::AttachThreadInput($me, $uiThread, $false) | Out-Null
  }
  function Shot($name) {
    $r = New-Object WD+RECT; [WD]::GetWindowRect($h, [ref]$r) | Out-Null
    $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $dc = $g.GetHdc(); [WD]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
    $bmp.Save((Join-Path $OutDir $name), [System.Drawing.Imaging.ImageFormat]::Png)
  }
  # a wide landscape picture: sky, sun, hills
  $pic = New-Object System.Drawing.Bitmap 960, 540
  $g = [System.Drawing.Graphics]::FromImage($pic)
  $g.SmoothingMode = 'AntiAlias'
  $sky = New-Object System.Drawing.Drawing2D.LinearGradientBrush (New-Object System.Drawing.Point 0, 0), (New-Object System.Drawing.Point 0, 540), ([System.Drawing.Color]::FromArgb(250, 190, 120)), ([System.Drawing.Color]::FromArgb(110, 170, 220))
  $g.FillRectangle($sky, 0, 0, 960, 540)
  $g.FillEllipse([System.Drawing.Brushes]::LemonChiffon, 400, 110, 150, 150)
  $hill = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(50, 70, 110))
  $g.FillPolygon($hill, @((New-Object System.Drawing.Point 0, 540), (New-Object System.Drawing.Point 300, 250), (New-Object System.Drawing.Point 520, 430), (New-Object System.Drawing.Point 700, 300), (New-Object System.Drawing.Point 960, 540)))
  $g.Dispose()
  [Windows.Forms.Clipboard]::SetImage($pic)
  CtrlV
  Shot "thumb-$Tag-image.png"
  # a tall picture replaces it
  $tall = New-Object System.Drawing.Bitmap 300, 900
  $g = [System.Drawing.Graphics]::FromImage($tall)
  $g.Clear([System.Drawing.Color]::FromArgb(30, 120, 90))
  $g.FillEllipse([System.Drawing.Brushes]::White, 75, 375, 150, 150)
  $g.Dispose()
  Start-Sleep -Milliseconds 1100
  [Windows.Forms.Clipboard]::SetImage($tall)
  CtrlV
  Shot "thumb-$Tag-tall.png"
  # then a file: the bar goes back to text only
  $note = Join-Path $OutDir "notatka-testowa.txt"
  [IO.File]::WriteAllText($note, "test wklejania pliku")
  $list = New-Object System.Collections.Specialized.StringCollection
  $list.Add($note) | Out-Null
  [Windows.Forms.Clipboard]::SetFileDropList($list)
  CtrlV
  Shot "thumb-$Tag-file.png"
  "w outbox po zastapieniu plikiem: {0}" -f @(Get-ChildItem $outbox -ErrorAction SilentlyContinue).Count
  $p.Refresh()
  "alive={0} pamiec={1:N1} MB" -f (-not $p.HasExited), ((Get-CimInstance Win32_PerfFormattedData_PerfProc_Process -Filter "IDProcess=$($p.Id)").WorkingSetPrivate/1MB)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  if ($keepText -ne $null) { [Windows.Forms.Clipboard]::SetText($keepText) }
  elseif ($keepImage -ne $null) { [Windows.Forms.Clipboard]::SetImage($keepImage) }
  elseif ($keepFiles -ne $null) { [Windows.Forms.Clipboard]::SetFileDropList($keepFiles) }
  else { [Windows.Forms.Clipboard]::Clear() }
  "schowek przywrocony"
}
