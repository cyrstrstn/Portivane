$ErrorActionPreference = 'Stop'
$installDir = Join-Path $env:ProgramFiles 'Portivane'
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
$download = 'https://github.com/cyrstrstn/Portivane/releases/latest/download/portivane.exe'
Invoke-WebRequest -Uri $download -OutFile (Join-Path $installDir 'portivane.exe')
$binary = Join-Path $installDir 'portivane.exe'
sc.exe stop Portivane 2>$null | Out-Null
sc.exe delete Portivane 2>$null | Out-Null
sc.exe create Portivane binPath= "`"$binary`"" start= auto DisplayName= "Portivane" | Out-Null
sc.exe description Portivane "Portivane local service publisher" | Out-Null
sc.exe start Portivane | Out-Null
Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name PortivaneTray -Value "powershell.exe -STA -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$installDir\portivane-tray.ps1`"" -Type String
Copy-Item (Join-Path $PSScriptRoot 'portivane-tray.ps1') (Join-Path $installDir 'portivane-tray.ps1') -Force
Start-Process 'http://127.0.0.1:4747'
Write-Host 'Portivane installed as a Windows service and tray app.'
