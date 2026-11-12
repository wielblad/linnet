param([string]$Exe, [string]$DataDir, [string]$OutDir, [string]$Python = "python", [string]$Name = "Linnet Test")
# Installs under a test name, checks what was made, shows a notification from the installed
# copy, then uninstalls and checks that nothing is left.
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class WG {
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint m, IntPtr w, IntPtr l);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr p, IntPtr a, string c, string t);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
}
"@
[WG]::SetProcessDPIAware() | Out-Null
$slug = $Name.ToLower().Replace(" ", "-")
$id = $Name.Replace(" ", "")
$dir = Join-Path $env:LOCALAPPDATA "Programs\$Name"
$lnk = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\$Name.lnk"
$key = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\$id"
function State($when) {
  "{0}: folder={1} exe={2} ikona={3} skrot={4} wpis={5}" -f $when, (Test-Path $dir), (Test-Path (Join-Path $dir "$slug.exe")), (Test-Path (Join-Path $dir "$slug.ico")), (Test-Path $lnk), (Test-Path $key)
}
State "przed"
$i = Start-Process -FilePath $Exe -ArgumentList @("-install", "-quiet", "-name", "`"$Name`"") -PassThru -Wait
"instalator zakonczyl sie kodem {0}" -f $i.ExitCode
State "po instalacji"
if (Test-Path $lnk) {
  $s = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
  "skrot: cel={0} | ikona={1} | opis={2}" -f (Split-Path $s.TargetPath -Leaf), (Split-Path ($s.IconLocation -replace ',.*$','') -Leaf), $s.Description
  $app = Get-StartApps | Where-Object { $_.Name -eq $Name }
  "menu Start: nazwa={0} identyfikator={1}" -f $app.Name, $app.AppID
}
if (Test-Path $key) {
  $r = Get-ItemProperty $key
  "wpis: {0} {1} | odinstalowanie: {2} | rozmiar {3} KB" -f $r.DisplayName, $r.DisplayVersion, ($r.UninstallString -replace [regex]::Escape($env:LOCALAPPDATA), '%LOCALAPPDATA%'), $r.EstimatedSize
}
if (Test-Path $lnk) {
  # the icon Windows itself shows for the shortcut
  $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($lnk)
  $icon.ToBitmap().Save((Join-Path $OutDir "ikona-skrotu.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "ikona skrotu: {0}x{1}" -f $icon.Width, $icon.Height
}
# a notification from the installed copy
$settings = '{"toasts":true,"body":true,"background":true,"show_mem":true,"dnd":false,"head_preview":false,"heads":[]}'
[IO.File]::WriteAllText((Join-Path $DataDir "settings.json"), $settings)
$installed = Join-Path $dir "$slug.exe"
$p = Start-Process -FilePath $installed -ArgumentList @("-data-dir", "`"$DataDir`"", "-self", "999", "-offline", "-tray", "-name", "`"$Name`"") -PassThru
Start-Sleep -Seconds 3
$h = [IntPtr]::Zero; $w = [IntPtr]::Zero
for ($n = 0; $n -lt 300; $n++) {
  $w = [WG]::FindWindowEx([IntPtr]::Zero, $w, "MliteMain", [NullString]::Value)
  if ($w -eq [IntPtr]::Zero) { break }
  $wpid = 0; [WG]::GetWindowThreadProcessId($w, [ref]$wpid) | Out-Null
  if ($wpid -eq $p.Id) { $h = $w; break }
}
if ($h -ne [IntPtr]::Zero) {
  & $Python -I (Join-Path $OutDir "newmsg.py") $DataDir | Out-Null
  [WG]::PostMessage($h, 0x8001, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
  Start-Sleep -Milliseconds 2500
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $bmp = New-Object System.Drawing.Bitmap 400, 190
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($b.Right - 400, $b.Bottom - 250, 0, 0, $bmp.Size)
  $g.Dispose()
  $bmp.Save((Join-Path $OutDir "toast-installed.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  "powiadomienie: zrzut zapisany"
} else { "powiadomienie: brak okna zainstalowanej kopii" }
Remove-Item (Join-Path $DataDir "settings.json") -ErrorAction SilentlyContinue
# uninstall while the installed copy is still running: it must be asked to quit
$u = Start-Process -FilePath $installed -ArgumentList @("-uninstall", "-quiet", "-name", "`"$Name`"") -PassThru -Wait
"odinstalowanie zakonczylo sie kodem {0}" -f $u.ExitCode
Start-Sleep -Seconds 6
$p.Refresh()
"zainstalowana kopia nadal dziala: {0}" -f (-not $p.HasExited)
State "po odinstalowaniu"
"autostart o tej nazwie: {0}" -f ((Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -ErrorAction SilentlyContinue).PSObject.Properties.Name -contains $Name)
