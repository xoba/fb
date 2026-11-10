# Print labels for an in-memory file catalog.
files = [
    ["notes.md", 120],
    ["letter.eml", 240]
]

total = 0
for entry in files
    see label(entry) + nl
    total += entry[2]
next
see "total: " + total + " bytes" + nl

func label entry
    return entry[1] + ": " + entry[2] + " bytes"
