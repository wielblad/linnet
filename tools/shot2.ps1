param([string]$Exe, [string]$DataDir, [string]$Out, [int]$Select = -1, [int]$Nav = -1, [string]$Extra = "", [int]$Width = 0, [int]$Height = 0, [string]$Type = "", [switch]$Enter)
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class W2 {
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, string l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int hh, bool r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[W2]::SetProcessDPIAware() | Out-Null
$argList = @("-data-dir", "`"$DataDir`"")
if ($Extra -ne "") { $argList += $Extra.Split(" ") }
$p = Start-Process -FilePath $Exe -ArgumentList $argList -PassThru
Start-Sleep -Seconds 3
$p.Refresh()
$h = $p.MainWindowHandle
if ($h -eq [IntPtr]::Zero) { "no main window; exited=$($p.HasExited)"; exit 1 }
if ($Width -gt 0) {
  $r0 = New-Object W2+RECT
  [W2]::GetWindowRect($h, [ref]$r0) | Out-Null
  [W2]::MoveWindow($h, $r0.L, $r0.T, $Width, $Height, $true) | Out-Null
  Start-Sleep -Milliseconds 400
}
if ($Select -ge 0) {
  $lb = [W2]::GetDlgItem($h, 311)
  [W2]::SendMessage($lb, 0x0186, [IntPtr]$Select, [IntPtr]::Zero) | Out-Null
  [W2]::SendMessage($h, 0x0111, [IntPtr]((1 -shl 16) -bor 311), $lb) | Out-Null
  Start-Sleep -Seconds 1
}
if ($Enter) {
  [W2]::PostMessage([W2]::GetDlgItem($h, 311), 0x0100, [IntPtr]0x0D, [IntPtr]1) | Out-Null
  Start-Sleep -Milliseconds 700
}
if ($Type -ne "") {
  $ed = [W2]::GetDlgItem($h, 330)
  [W2]::SendMessage($ed, 0x00C2, [IntPtr]1, $Type) | Out-Null; Start-Sleep -Milliseconds 300
}
if ($Nav -ge 0) {
  [W2]::SendMessage($h, 0x0111, [IntPtr](301 + $Nav), [IntPtr]::Zero) | Out-Null
  Start-Sleep -Seconds 1
}
Start-Sleep -Milliseconds 600
$r = New-Object W2+RECT
[W2]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
[W2]::PrintWindow($h, $dc, 2) | Out-Null
$g.ReleaseHdc($dc)
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$p.Refresh()
"size={0}x{1} WS={2:N1}MB Private={3:N1}MB" -f $bmp.Width, $bmp.Height, ($p.WorkingSet64/1MB), ($p.PrivateMemorySize64/1MB)
Stop-Process -Id $p.Id -Force
