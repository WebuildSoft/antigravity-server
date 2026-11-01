; ==============================================================================
; Antigravity Server - Inno Setup Installer Script
; ==============================================================================
#define MyAppName "Antigravity Server"
#define MyAppVersion "1.4.0"
#define MyAppPublisher "WebuildSoft"
#define MyAppURL "https://webuildsoft.github.io/antigravity-mobile-site/"
#define MyAppExeName "antigravity-server.exe"

[Setup]
AppId={{5F8A2D1C-8A9E-4C23-9F6D-1E4E7A9C2B81}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=..\dist
OutputBaseFilename=AntigravityServer-Setup-v{#MyAppVersion}
SetupIconFile=..\app_icon.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "autostart"; Description: "Iniciar o Antigravity Server automaticamente com o Windows"; GroupDescription: "Inicializacao:"

[Files]
; Binário nativo Windows Go de alto desempenho
Source: "..\bin\antigravity-server-windows-amd64.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion

; Interface Flutter Web (Servida para o Celular)
Source: "..\web\*"; DestDir: "{app}\web"; Flags: ignoreversion recursesubdirs createallsubdirs

; Scripts auxiliares de inicialização
Source: "..\start.bat"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\start.ps1"; DestDir: "{app}"; Flags: ignoreversion

; Ícone do aplicativo
Source: "..\app_icon.ico"; DestDir: "{app}"; Flags: ignoreversion

; Cloudflared opcional se presente
Source: "..\bin\cloudflared.exe"; DestDir: "{app}\bin"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\app_icon.ico"; Parameters: "-port 4040"
Name: "{group}\{#MyAppName} (Console)"; Filename: "{app}\start.bat"; IconFilename: "{app}\app_icon.ico"
Name: "{group}\Desinstalar {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\app_icon.ico"; Tasks: desktopicon

[Registry]
; Inicialização automática com o Windows se selecionado
Root: HKLM; Subkey: "SOFTWARE\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "{#MyAppName}"; ValueData: """{app}\{#MyAppExeName}"""; Flags: uninsdeletevalue; Tasks: autostart

[Run]
; 1. Liberar Porta 4040 no Firewall do Windows automaticamente na instalação
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall add rule name=""Antigravity Server 4040"" dir=in action=allow protocol=TCP localport=4040 profile=any"; Flags: runhidden; StatusMsg: "Configurando regras no Firewall do Windows..."

; 2. Opção de iniciar o aplicativo após a conclusão
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; Remover regra do Firewall na desinstalação
Filename: "{sys}\netsh.exe"; Parameters: "advfirewall firewall delete rule name=""Antigravity Server 4040"""; Flags: runhidden
