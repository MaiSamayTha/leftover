; Leftover installer for Windows, built with Inno Setup 6 by
; scripts/package-windows.ps1. Installs for the current user only, so it
; needs no admin rights, the same place the PowerShell install script uses.
;   iscc /DAppVersion=0.1.1 /DBuildDir=<folder with the exes> /DOutDir=<out> leftover.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppId={{6F1C0C8E-3B9A-4C55-9C7E-1E7A2D5B9F31}
AppName=Leftover
AppVersion={#AppVersion}
AppVerName=Leftover {#AppVersion}
AppPublisher=Leftover contributors
AppPublisherURL=https://maisamaytha.github.io/leftover/
AppSupportURL=https://github.com/MaiSamayTha/leftover/issues
AppUpdatesURL=https://github.com/MaiSamayTha/leftover/releases
DefaultDirName={localappdata}\Programs\Leftover
DisableDirPage=yes
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64 arm64
ArchitecturesInstallIn64BitMode=x64 arm64
OutputDir={#OutDir}
OutputBaseFilename=Leftover-Setup
SetupIconFile=..\..\assets\leftover.ico
UninstallDisplayIcon={app}\leftover.ico
UninstallDisplayName=Leftover
LicenseFile=..\..\LICENSE
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ChangesEnvironment=yes
CloseApplications=yes

[Files]
Source: "{#BuildDir}\amd64\leftover.exe"; DestDir: "{app}"; Flags: ignoreversion; Check: not IsArm64
Source: "{#BuildDir}\amd64\leftoverw.exe"; DestDir: "{app}"; Flags: ignoreversion; Check: not IsArm64
Source: "{#BuildDir}\arm64\leftover.exe"; DestDir: "{app}"; Flags: ignoreversion; Check: IsArm64
Source: "{#BuildDir}\arm64\leftoverw.exe"; DestDir: "{app}"; Flags: ignoreversion; Check: IsArm64
Source: "..\..\assets\leftover.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion

[Tasks]
Name: "desktopicon"; Description: "Add a shortcut to the desktop"; Flags: unchecked
Name: "addtopath"; Description: "Add the 'leftover' command to the terminal (PATH)"

[Icons]
Name: "{userprograms}\Leftover"; Filename: "{app}\leftoverw.exe"; IconFilename: "{app}\leftover.ico"; Comment: "See what your dev tools left behind and clear it safely"
Name: "{userdesktop}\Leftover"; Filename: "{app}\leftoverw.exe"; IconFilename: "{app}\leftover.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\leftoverw.exe"; Description: "Open Leftover now"; Flags: nowait postinstall skipifsilent
; Leftover's own Update now runs this installer silently with /LAUNCH=1, so
; it opens again once the new version is in place.
Filename: "{app}\leftoverw.exe"; Flags: nowait; Check: LaunchAfterUpdate

[Code]
const
  EnvKey = 'Environment';

function LaunchAfterUpdate: Boolean;
begin
  Result := WizardSilent and (ExpandConstant('{param:LAUNCH|0}') = '1');
end;

function PathHas(Path, Dir: string): Boolean;
begin
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(Path) + ';') > 0;
end;

procedure AddToPath(Dir: string);
var
  Path: string;
begin
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then
    Path := '';
  if PathHas(Path, Dir) then
    exit;
  if (Path <> '') and (Copy(Path, Length(Path), 1) <> ';') then
    Path := Path + ';';
  RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Path + Dir);
end;

procedure RemoveFromPath(Dir: string);
var
  Path: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKCU, EnvKey, 'Path', Path) then
    exit;
  Path := ';' + Path + ';';
  P := Pos(';' + Uppercase(Dir) + ';', Uppercase(Path));
  if P = 0 then
    exit;
  Delete(Path, P, Length(Dir) + 1);
  Path := Copy(Path, 2, Length(Path) - 2);
  RegWriteExpandStringValue(HKCU, EnvKey, 'Path', Path);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and WizardIsTaskSelected('addtopath') then
    AddToPath(ExpandConstant('{app}'));
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;
