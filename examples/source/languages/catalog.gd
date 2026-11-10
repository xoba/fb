extends Node
# Print labels for an in-memory file catalog.

const FILES = [
    {"name": "notes.md", "bytes": 120},
    {"name": "letter.eml", "bytes": 240},
]


func label(entry):
    return "%s: %d bytes" % [entry["name"], entry["bytes"]]


func _ready():
    for entry in FILES:
        print(label(entry))
