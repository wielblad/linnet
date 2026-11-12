param([string]$Exe, [string]$DataDir, [string]$OutDir)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class W9 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[W9]::SetProcessDPIAware() | Out-Null
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$p.Refresh()
$h = $p.MainWindowHandle
$list = [W9]::GetDlgItem($h, 311)
$edit = [W9]::GetDlgItem($h, 330)
function State($label) {
  Start-Sleep -Milliseconds 500
  "{0}: lista widoczna={1}, pole pisania widoczne={2}" -f $label, [W9]::IsWindowVisible($list), [W9]::IsWindowVisible($edit)
}
function Shot($name) {
  $r = New-Object W9+RECT
  [W9]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [W9]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
}
# wide: open a conversation, then make the window narrow: the conversation stays in view
[W9]::SendMessage($list, 0x0186, [IntPtr]1, [IntPtr]::Zero) | Out-Null
[W9]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $list) | Out-Null
State "szerokie okno z rozmowa"
$r0 = New-Object W9+RECT
[W9]::GetWindowRect($h, [ref]$r0) | Out-Null
[W9]::MoveWindow($h, $r0.L, $r0.T, 400, 760, $true) | Out-Null
State "po zwezeniu"
# back arrow
[W9]::PostMessage($h, 0x0202, [IntPtr]::Zero, [IntPtr]((25 -shl 16) -bor 20)) | Out-Null
State "po strzalce wstecz"
# arrow keys move in the list without opening, Enter opens
[W9]::PostMessage($list, 0x0100, [IntPtr]0x28, [IntPtr]1) | Out-Null
State "po strzalce w dol na liscie"
[W9]::PostMessage($list, 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null
State "po Enter"
Shot "n7"
# Esc in the text field goes back
[W9]::PostMessage($edit, 0x0100, [IntPtr]0x1B, [IntPtr]1) | Out-Null
State "po Esc"
# wide again: both columns
[W9]::MoveWindow($h, $r0.L, $r0.T, 1020, 700, $true) | Out-Null
State "po poszerzeniu"
Stop-Process -Id $p.Id -Force
