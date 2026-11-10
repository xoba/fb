; Install the example catalog into a folder of the user's choosing.
Name "Example Catalog"
OutFile "catalog-setup.exe"
InstallDir "$PROFILE\Example Catalog"
RequestExecutionLevel user

Page directory
Page instfiles

Section "Catalog"
  SetOutPath "$INSTDIR"
  File "notes.md"
  File "letter.eml"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  DetailPrint "Installed 2 files (360 bytes)"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\notes.md"
  Delete "$INSTDIR\letter.eml"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
