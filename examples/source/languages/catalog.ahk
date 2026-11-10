; Show labels for an in-memory file catalog.
files := [{name: "notes.md", bytes: 120}, {name: "letter.eml", bytes: 240}]
labels := ""

for index, entry in files
    labels .= entry.name ": " entry.bytes " bytes`n"

MsgBox, % labels
