; KeeYGeeN桌面助手2 安装脚本（Inno Setup 6）
; 编译：ISCC.exe installer\KeeYGeeN桌面助手2.iss

#define MyAppName "KeeYGeeN桌面助手2"
#define MyAppVersion "2.2.2"
#define MyAppPublisher "KeeYGeeN"
#define MyAppExeName "KeeYGeeN桌面助手2.exe"

[Setup]
AppId={{A7C4E1F2-6B39-4D58-9E71-2F3A8C5D4B10}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=..\dist
OutputBaseFilename=KeeYGeeN桌面助手2-Setup
SetupIconFile=..\build\windows\icon.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
UninstallDisplayName={#MyAppName}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
DisableWelcomePage=no
WizardImageFile=assets2\wizard.bmp
WizardSmallImageFile=assets2\wizard-small.bmp
WizardImageStretch=no
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=yes
RestartApplications=no
AllowNoIcons=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务："; Flags: checkedonce

[Files]
Source: "..\build\bin\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "assets2\bg.png"; DestDir: "{app}"; DestName: "bg.png"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "启动 {#MyAppName}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
Type: filesandordirs; Name: "{app}"
