param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Python = "python")
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
Add-Type -ReferencedAssemblies System.Drawing @"
using System;
using System.Runtime.InteropServices;
public class WA {
  [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
  [DllImport("user32.dll")] public static extern IntPtr GetDlgItem(IntPtr h, int id);
  [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr h);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public struct RECT { public int L, T, R, B; }
}
"@
[WA]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":false,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":true,"heads":[{"id":102,"x":1500,"y":300}]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline") -PassThru
Start-Sleep -Seconds 3
function Find($cls) {
  $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WA]::FindWindowEx([IntPtr]::Zero, $w, $cls, [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { return [IntPtr]::Zero }
    $wpid = 0; [WA]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { return $w }
  }
  return [IntPtr]::Zero
}
$h = Find "MliteMain"
$list = [WA]::GetDlgItem($h, 311)
# 1. what a screen reader is told when the keyboard moves through the list
foreach ($i in 0, 1) {
  [WA]::PostMessage($list, 0x0100, [IntPtr]0x28, [IntPtr]1) | Out-Null
  Start-Sleep -Milliseconds 500
  $name = [System.Windows.Automation.AutomationElement]::FromHandle($list).Current.Name
  "czytnik ekranu, wiersz {0}: dlugosc nazwy {1}, zaczyna sie od: {2}" -f ($i + 1), $name.Length, $name.Substring(0, [Math]::Min(28, $name.Length))
}
# 2. a new message with emoji: the preview beside the head
& $Python -I (Join-Path $OutDir "newmsg.py") $DataDir | Out-Null
[WA]::PostMessage($h, 0x8001, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Start-Sleep -Milliseconds 1200
$peek = Find "MlitePeek"
if ($peek -ne [IntPtr]::Zero) {
  $r = New-Object WA+RECT; [WA]::GetWindowRect($peek, [ref]$r) | Out-Null
  $bmp = New-Object System.Drawing.Bitmap ($r.R - $r.L), ($r.B - $r.T)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $dc = $g.GetHdc(); [WA]::PrintWindow($peek, $dc, 2) | Out-Null; $g.ReleaseHdc($dc); $g.Dispose()
  $bmp.Save((Join-Path $OutDir "peek.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "dymek przy glowce: {0}x{1}" -f $bmp.Width, $bmp.Height
} else { "dymek przy glowce: brak okna" }
$p.Refresh()
"alive={0}" -f (-not $p.HasExited)
Stop-Process -Id $p.Id -Force
Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
