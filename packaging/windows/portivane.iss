#define MyAppName "Portivane"
#define MyAppVersion "0.4.2"
#define MyAppExeName "portivane.exe"
[Setup]
AppId={{A58F15DA-2D74-4CFB-9E2A-PORTIVANE}}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
DefaultDirName={autopf}\Portivane
DefaultGroupName=Portivane
OutputBaseFilename=portivane-windows-installer
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}
[Files]
Source: "..\..\release\portivane.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "portivane-tray.ps1"; DestDir: "{app}"; Flags: ignoreversion
[Run]
Filename: "{app}\{#MyAppExeName}"; Parameters: "-service-install"; Flags: runhidden waituntilterminated
Filename: "sc.exe"; Parameters: "start Portivane"; Flags: runhidden waituntilterminated
Filename: "http://127.0.0.1:4747"; Flags: shellexec nowait postinstall
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "PortivaneTray"; ValueData: "powershell.exe -STA -WindowStyle Hidden -ExecutionPolicy Bypass -File ""{app}\portivane-tray.ps1"""; Flags: uninsdeletevalue
[UninstallRun]
Filename: "{app}\{#MyAppExeName}"; Parameters: "-service-uninstall"; Flags: runhidden waituntilterminated
