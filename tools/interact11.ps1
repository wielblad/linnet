param([string]$Exe, [string]$DataDir, [string]$OutDir)
# Pasted picture is removed from the outbox when the attachment is given up (Esc / the close mark).
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WC {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint a, uint b, bool f);
  [DllImport("user32.dll")] public static extern bool GetKeyboardState(byte[] s);
  [DllImport("user32.dll")] public static extern bool SetKeyboardState(byte[] s);
  [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
}
"@
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
    $w = [WC]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; $tid = [WC]::GetWindowThreadProcessId($w, [ref]$wpid)
    if ($wpid -eq $p.Id) { $h = $w; $uiThread = $tid; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  $lb = [WC]::GetDlgItem($h, 311)
  [WC]::SendMessage($lb, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
  [WC]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 1
  $edit = [WC]::GetDlgItem($h, 330)
  function Key($vk, $ctrl) {
    $me = [WC]::GetCurrentThreadId()
    [WC]::AttachThreadInput($me, $uiThread, $true) | Out-Null
    $st = New-Object byte[] 256
    [WC]::GetKeyboardState($st) | Out-Null
    if ($ctrl) { $st[0x11] = 0x80 } else { $st[0x11] = 0 }
    [WC]::SetKeyboardState($st) | Out-Null
    [WC]::PostMessage($edit, 0x0100, [IntPtr]$vk, [IntPtr]1) | Out-Null
    Start-Sleep -Milliseconds 700
    $st[0x11] = 0
    [WC]::SetKeyboardState($st) | Out-Null
    [WC]::AttachThreadInput($me, $uiThread, $false) | Out-Null
  }
  function Count { @(Get-ChildItem $outbox -ErrorAction SilentlyContinue).Count }
  $pic = New-Object System.Drawing.Bitmap 64, 64
  [Windows.Forms.Clipboard]::SetImage($pic)
  Key 0x56 $true
  "po wklejeniu pierwszego obrazka: {0}" -f (Count)
  Start-Sleep -Milliseconds 1100
  Key 0x56 $true
  "po wklejeniu drugiego (zastapil pierwszy): {0}" -f (Count)
  Key 0x1B $false
  "po Esc: {0}" -f (Count)
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
