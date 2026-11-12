param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$FilesDir)
# A picture file from the disk (the same path the file dialog takes) shows its thumbnail above the text field.
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WE {
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
[WE]::SetProcessDPIAware() | Out-Null
$keepText = $null; $keepImage = $null; $keepFiles = $null
if ([Windows.Forms.Clipboard]::ContainsText()) { $keepText = [Windows.Forms.Clipboard]::GetText() }
elseif ([Windows.Forms.Clipboard]::ContainsImage()) { $keepImage = [Windows.Forms.Clipboard]::GetImage() }
elseif ([Windows.Forms.Clipboard]::ContainsFileDropList()) { $keepFiles = [Windows.Forms.Clipboard]::GetFileDropList() }
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
try {
  Start-Sleep -Seconds 3
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WE]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; $tid = [WE]::GetWindowThreadProcessId($w, [ref]$wpid)
    if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  $lb = [WE]::GetDlgItem($h, 311)
  [WE]::SendMessage($lb, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
  [WE]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 1
  $edit = [WE]::GetDlgItem($h, 330)
  function CtrlV {
    $me = [WE]::GetCurrentThreadId()
    [WE]::AttachThreadInput($me, $uiThread, $true) | Out-Null
    $st = New-Object byte[] 256
    [WE]::GetKeyboardState($st) | Out-Null
    $st[0x11] = 0x80
    [WE]::SetKeyboardState($st) | Out-Null
    [WE]::PostMessage($edit, 0x0100, [IntPtr]0x56, [IntPtr]1) | Out-Null
    Start-Sleep -Milliseconds 900
    $st[0x11] = 0
    [WE]::SetKeyboardState($st) | Out-Null
    [WE]::AttachThreadInput($me, $uiThread, $false) | Out-Null
  }
  foreach ($f in Get-ChildItem $FilesDir -File | Sort-Object Name) {
    $list = New-Object System.Collections.Specialized.StringCollection
    $list.Add($f.FullName) | Out-Null
    [Windows.Forms.Clipboard]::SetFileDropList($list)
    CtrlV
    $r = New-Object WE+RECT; [WE]::GetWindowRect($h, [ref]$r) | Out-Null
    $full = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
    $g = [System.Drawing.Graphics]::FromImage($full)
    $dc = $g.GetHdc(); [WE]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
    # only the composer: the bottom 150 px of the conversation column
    $cut = New-Object System.Drawing.Rectangle 376, ($full.Height - 150), ($full.Width - 376 - 8), 142
    $part = $full.Clone($cut, $full.PixelFormat)
    $part.Save((Join-Path $OutDir ("disk-" + $f.BaseName + ".png")), [System.Drawing.Imaging.ImageFormat]::Png)
    "{0}: nadal na dysku={1}" -f $f.Name, (Test-Path $f.FullName)
  }
  "pliki obok wybranych: {0}" -f ((Get-ChildItem $FilesDir -File | Select-Object -ExpandProperty Name) -join ", ")
  "outbox: {0}" -f @(Get-ChildItem (Join-Path $DataDir "outbox") -ErrorAction SilentlyContinue).Count
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
