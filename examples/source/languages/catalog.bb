; Print labels for an in-memory file catalog.
Type Entry
	Field name$
	Field bytes%
End Type

Function AddEntry(name$, bytes%)
	Local e.Entry = New Entry
	e\name$ = name$
	e\bytes% = bytes%
End Function

AddEntry("notes.md", 120)
AddEntry("letter.eml", 240)

For e.Entry = Each Entry
	Print e\name$ + ": " + e\bytes% + " bytes"
Next

WaitKey()
End
