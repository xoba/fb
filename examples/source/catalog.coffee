files = [
  {name: "notes.md", bytes: 120}
  {name: "letter.eml", bytes: 240}
]

for file in files
  console.log "#{file.name}: #{file.bytes} bytes"
