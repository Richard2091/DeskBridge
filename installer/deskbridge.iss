; DeskBridge 安装程序（Inno Setup 6）。
; 编译：iscc /DAppVersion=0.1.0 /DAppNumVersion=0.1.0.0 /DSourceDir=<Release 目录> /O<输出目录> /F<文件名> installer\deskbridge.iss
; 按当前用户安装，不需要管理员权限。

#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#ifndef AppNumVersion
  #define AppNumVersion "0.0.0.0"
#endif
#ifndef SourceDir
  #error 请用 /DSourceDir= 指定 Release 目录（含 deskbridge_client.exe 和 deskbridge.exe）
#endif

[Setup]
; 固定的 AppId，升级和卸载靠它识别；不要修改。
AppId={{5D0E7B3A-4C1F-4E8B-9A62-3F7C1B8D2E94}
AppName=DeskBridge
AppVersion={#AppVersion}
AppPublisher=DeskBridge
DefaultDirName={localappdata}\Programs\DeskBridge
DefaultGroupName=DeskBridge
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
Compression=lzma2
SolidCompression=yes
OutputBaseFilename=DeskBridge-setup
VersionInfoVersion={#AppNumVersion}
VersionInfoProductName=DeskBridge
UninstallDisplayIcon={app}\deskbridge_client.exe
; 客户端和 ui-core 子进程可能在运行，升级时强制关闭；装完不自动重启它们。
CloseApplications=force
RestartApplications=no

[Languages]
Name: "zh"; MessagesFile: "ChineseSimplified.isl"

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; Flags: unchecked
Name: "autostart"; Description: "登录后在后台运行 DeskBridge 节点（关闭客户端后本机仍可被远程控制）"; Flags: unchecked

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\DeskBridge"; Filename: "{app}\deskbridge_client.exe"
Name: "{autodesktop}\DeskBridge"; Filename: "{app}\deskbridge_client.exe"; Tasks: desktopicon

[Run]
; 勾选自启动时安装计划任务（install 默认会立即启动节点）。
Filename: "{app}\deskbridge.exe"; Parameters: "install"; StatusMsg: "正在安装后台节点..."; Flags: runhidden waituntilterminated; Tasks: autostart
Filename: "{app}\deskbridge_client.exe"; Description: "立即运行 DeskBridge"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; 没装过自启动时 uninstall 会失败；UninstallRun 不检查退出码，失败不影响卸载。
Filename: "{app}\deskbridge.exe"; Parameters: "uninstall"; RunOnceId: "RemoveAutostart"; Flags: runhidden waituntilterminated
