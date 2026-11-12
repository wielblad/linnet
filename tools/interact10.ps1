param([string]$Exe, [string]$DataDir, [string]$OutDir)
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WB {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
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
[WB]::SetProcessDPIAware() | Out-Null

# keep what is on the clipboard now (never printed), to put it back at the end
$keepText = $null; $keepImage = $null; $keepFiles = $null
if ([Windows.Forms.Clipboard]::ContainsText()) { $keepText = [Windows.Forms.Clipboard]::GetText() }
elseif ([Windows.Forms.Clipboard]::ContainsImage()) { $keepImage = [Windows.Forms.Clipboard]::GetImage() }
elseif ([Windows.Forms.Clipboard]::ContainsFileDropList()) { $keepFiles = [Windows.Forms.Clipboard]::GetFileDropList() }
"schowek przed testem: tekst={0} obraz={1} pliki={2}" -f ($keepText -ne $null), ($keepImage -ne $null), ($keepFiles -ne $null)

$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 3
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WB]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; $tid = [WB]::GetWindowThreadProcessId($w, [ref]$wpid)
    if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  $lb = [WB]::GetDlgItem($h, 311)
  [WB]::SendMessage($lb, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
  [WB]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 1
  $edit = [WB]::GetDlgItem($h, 330)

  function CtrlV {
    $me = [WB]::GetCurrentThreadId()
    [WB]::AttachThreadInput($me, $uiThread, $true) | Out-Null
    $st = New-Object byte[] 256
    [WB]::GetKeyboardState($st) | Out-Null
    $st[0x11] = 0x80
    [WB]::SetKeyboardState($st) | Out-Null
    [WB]::PostMessage($edit, 0x0100, [IntPtr]0x56, [IntPtr]1) | Out-Null
    Start-Sleep -Milliseconds 700
    $st[0x11] = 0
    [WB]::SetKeyboardState($st) | Out-Null
    [WB]::AttachThreadInput($me, $uiThread, $false) | Out-Null
  }
  function Shot($name) {
    $r = New-Object WB+RECT; [WB]::GetWindowRect($h, [ref]$r) | Out-Null
    $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $dc = $g.GetHdc(); [WB]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
    $bmp.Save((Join-Path $OutDir $name), [System.Drawing.Imaging.ImageFormat]::Png)
  }
  $outbox = Join-Path $DataDir "outbox"
  $before = @(Get-ChildItem $outbox -ErrorAction SilentlyContinue).Count

  # 1. a picture
  $pic = New-Object System.Drawing.Bitmap 320, 200
  $g = [System.Drawing.Graphics]::FromImage($pic)
  $g.Clear([System.Drawing.Color]::FromArgb(40, 90, 160))
  $g.FillEllipse([System.Drawing.Brushes]::Gold, 110, 50, 100, 100)
  $g.Dispose()
  [Windows.Forms.Clipboard]::SetImage($pic)
  CtrlV
  Shot "paste-image.png"
  $files = @(Get-ChildItem $outbox -ErrorAction SilentlyContinue | Sort-Object LastWriteTime)
  "obraz: plikow w outbox przed {0}, po {1}" -f $before, $files.Count
  if ($files.Count -gt $before) {
    $last = $files[-1]
    $img = [System.Drawing.Image]::FromFile($last.FullName)
    "obraz: zapisany {0}, {1}x{2}, {3} B" -f $last.Name, $img.Width, $img.Height, $last.Length
    $img.Dispose()
  }

  # 2. a file copied in Explorer
  $note = Join-Path $OutDir "notatka-testowa.txt"
  [IO.File]::WriteAllText($note, "test wklejania pliku")
  $list = New-Object System.Collections.Specialized.StringCollection
  $list.Add($note) | Out-Null
  [Windows.Forms.Clipboard]::SetFileDropList($list)
  CtrlV
  Shot "paste-file.png"

  # 3. plain text still goes into the field
  [Windows.Forms.Clipboard]::SetText("zwykly tekst ze schowka")
  CtrlV
  Shot "paste-text.png"
  $len = [WB]::SendMessage($edit, 0x000E, [IntPtr]::Zero, [IntPtr]::Zero)
  "tekst: dlugosc pola po wklejeniu {0}" -f $len
  $p.Refresh()
  "alive={0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  if ($keepText -ne $null) { [Windows.Forms.Clipboard]::SetText($keepText) }
  elseif ($keepImage -ne $null) { [Windows.Forms.Clipboard]::SetImage($keepImage) }
  elseif ($keepFiles -ne $null) { [Windows.Forms.Clipboard]::SetFileDropList($keepFiles) }
  else { [Windows.Forms.Clipboard]::Clear() }
  "schowek przywrocony"
}
