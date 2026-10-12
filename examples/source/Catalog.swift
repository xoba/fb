struct Entry {
    let name: String
    let bytes: Int
}

let files = [
    Entry(name: "notes.md", bytes: 120),
    Entry(name: "letter.eml", bytes: 240),
]

for file in files {
    print("\(file.name): \(file.bytes) bytes")
}
