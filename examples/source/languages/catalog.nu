# Print labels for an in-memory file catalog.
let files = [
    [name bytes];
    ["notes.md" 120]
    ["letter.eml" 240]
]

def label [entry: record] {
    $"($entry.name): ($entry.bytes) bytes"
}

for entry in $files {
    print (label $entry)
}

let total = $files | get bytes | math sum
print $"total: ($total) bytes"
