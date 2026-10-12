"""Print labels for an in-memory file catalog."""

files = [
    {"name": "notes.md", "bytes": 120},
    {"name": "letter.eml", "bytes": 240},
]

for entry in files:
    print(f"{entry['name']}: {entry['bytes']} bytes")
