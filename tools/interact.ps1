param([string]$Exe, [string]$DataDir, [string]$OutDir)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class W4 {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, string l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr h, IntPtr after, int x, int y, int cx, int cy, uint flags);
  [DllImport("user32.dll")] public static extern IntPtr GetFocus();
  public struct RECT { public int L, T, R, B; }
}
"@
[W4]::SetProcessDPIAware() | Out-Null
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
$p.Refresh()
$h = $p.MainWindowHandle
if ($h -eq [IntPtr]::Zero) { "no main window"; exit 1 }
$list = [W4]::GetDlgItem($h, 311)
$edit = [W4]::GetDlgItem($h, 330)
function Shot($name, [switch]$Screen) {
  Start-Sleep -Milliseconds 500
  $r = New-Object W4+RECT
  [W4]::GetWindowRect($h, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  if ($Screen) {
    $g.CopyFromScreen($r.L, $r.T, 0, 0, $bmp.Size)
  } else {
    $dc = $g.GetHdc(); [W4]::PrintWindow($h, $dc, 2) | Out-Null; $g.ReleaseHdc($dc)
  }
  $g.Dispose()
  $bmp.Save((Join-Path $OutDir "$name.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "saved $name"
}
function MakeLP($x, $y) { [IntPtr](($y -shl 16) -bor $x) }

# 1. People tab, pointer over the third row
[W4]::SendMessage($h, 0x0111, [IntPtr]302, [IntPtr]::Zero) | Out-Null
Start-Sleep -Milliseconds 400
[W4]::PostMessage($list, 0x0200, [IntPtr]::Zero, (MakeLP 120 140)) | Out-Null
Shot "h1"
# 2. click that row: the conversation opens in the Chats tab
[W4]::PostMessage($list, 0x0201, [IntPtr]1, (MakeLP 120 140)) | Out-Null
[W4]::PostMessage($list, 0x0202, [IntPtr]::Zero, (MakeLP 120 140)) | Out-Null
Shot "h2"
# 3. type and press Enter with no connection: the text must stay and a notice must appear
[W4]::SendMessage($edit, 0x00C2, [IntPtr]1, "Test wysyłki bez połączenia") | Out-Null
[W4]::PostMessage($edit, 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null
Shot "h3"
# 4. the real screen, to see the title bar colour
[W4]::SetWindowPos($h, [IntPtr](-1), 0, 0, 0, 0, 0x0043) | Out-Null
Shot "h4" -Screen
$p.Refresh()
"WS={0:N1}MB" -f ($p.WorkingSet64/1MB)
Stop-Process -Id $p.Id -Force
