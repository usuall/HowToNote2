# 내가 띄운 앱 창 하나를 PID로 찾아 PNG로 캡처한다(UI 결과 기록용). 바탕 화면을 사용자와 함께 쓰므로 이름이나
# '앞에 있는 창'으로 찾지 않고, 창을 앞으로 가져오지도 않는다. WebView2 내용까지 찍으려고 PrintWindow에
# PW_RENDERFULLCONTENT(2)를 준다.
param(
  [Parameter(Mandatory = $true)][int]$ProcessId,
  [Parameter(Mandatory = $true)][string]$Out
)

Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public static class CaptureWin {
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr hdc, uint flags);
}
'@

[CaptureWin]::SetProcessDPIAware() | Out-Null # 배율이 100%가 아니어도 물리 픽셀로 잰다
$h = (Get-Process -Id $ProcessId -ErrorAction Stop).MainWindowHandle
if ($h -eq [IntPtr]::Zero) { throw "PID $ProcessId 에 보이는 창이 없습니다." }
$r = New-Object CaptureWin+RECT
[CaptureWin]::GetWindowRect($h, [ref]$r) | Out-Null
$bmp = New-Object System.Drawing.Bitmap ($r.Right - $r.Left), ($r.Bottom - $r.Top)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$hdc = $g.GetHdc()
$ok = [CaptureWin]::PrintWindow($h, $hdc, 2)
$g.ReleaseHdc($hdc)
$g.Dispose()
if (-not $ok) { $bmp.Dispose(); throw "PrintWindow가 실패했습니다." }
$bmp.Save($Out, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output $Out
