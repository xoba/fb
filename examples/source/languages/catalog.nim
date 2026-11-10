# Print labels for an in-memory file catalog.
import std/strformat

type
  Entry = object
    name: string
    bytes: int

proc label(entry: Entry): string =
  &"{entry.name}: {entry.bytes} bytes"

let files = @[
  Entry(name: "notes.md", bytes: 120),
  Entry(name: "letter.eml", bytes: 240),
]

var total = 0
for entry in files:
  echo entry.label
  total += entry.bytes
echo "total: ", total, " bytes"
