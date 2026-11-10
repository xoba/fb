-- Print labels for an in-memory file catalog.
set catalogFiles to {{fileName:"notes.md", byteCount:120}, {fileName:"letter.eml", byteCount:240}}
set labels to {}

repeat with entry in catalogFiles
	set end of labels to (fileName of entry) & ": " & (byteCount of entry) & " bytes"
end repeat

set AppleScript's text item delimiters to linefeed
return labels as text
