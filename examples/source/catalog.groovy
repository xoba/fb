def files = [
    [name: 'notes.md', kind: 'document'],
    [name: 'letter.eml', kind: 'message']
]

files.each { file -> println "${file.name}: ${file.kind}" }
