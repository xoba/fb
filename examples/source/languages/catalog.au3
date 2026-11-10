; Show labels for an in-memory file catalog.
#include <MsgBoxConstants.au3>

Local $aNames[2] = ["notes.md", "letter.eml"]
Local $aBytes[2] = [120, 240]
Local $sLabels = ""

For $i = 0 To UBound($aNames) - 1
    $sLabels &= $aNames[$i] & ": " & $aBytes[$i] & " bytes" & @CRLF
Next

MsgBox($MB_OK, "Catalog", $sLabels)
