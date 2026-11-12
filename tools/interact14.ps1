param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Python = "python")
# A new message while the window is hidden: the Windows notification with the sender's picture.
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WF {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
}
"@
[WF]::SetProcessDPIAware() | Out-Null
$settings = '{"toasts":true,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"heads":[]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$env:MLITE_ICONDUMP = Join-Path $OutDir "ikony"
$p = Start-Process -FilePath $Exe -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline", "-tray") -PassThru
$env:MLITE_ICONDUMP = $null
try {
  Start-Sleep -Seconds 3
  $h = [IntPtr]::Zero; $w = [IntPtr]::Zero
  for ($i = 0; $i -lt 300; $i++) {
    $w = [WF]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
    if ($w -eq [IntPtr]::Zero) { break }
    $wpid = 0; [WF]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
    if ($wpid -eq $p.Id) { $h = $w; break }
  }
  if ($h -eq [IntPtr]::Zero) { "brak okna"; return }
  & $Python -I (Join-Path $OutDir "newmsg.py") $DataDir | Out-Null
  [WF]::PostMessage($h, 0x8001, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  foreach ($delay in 1500, 1500) {
    Start-Sleep -Milliseconds $delay
    $bw = [Math]::Min(520, $b.Width); $bh = [Math]::Min(330, $b.Height)
    $bmp = New-Object System.Drawing.Bitmap $bw, $bh
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($b.Right - $bw, $b.Bottom - $bh, 0, 0, $bmp.Size)
    $g.Dispose()
    $bmp.Save((Join-Path $OutDir ("toast-" + $delay + "-" + [DateTime]::Now.ToString("ssfff") + ".png")), [System.Drawing.Imaging.ImageFormat]::Png)
  }
  "ekran {0}x{1}" -f $b.Width, $b.Height
  $p.Refresh()
  "alive={0}" -f (-not $p.HasExited)
} finally {
  if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
  Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
}
