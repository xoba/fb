struct Entry {
    name: &'static str,
    bytes: u64,
}

fn main() {
    let files = [
        Entry { name: "notes.md", bytes: 120 },
        Entry { name: "letter.eml", bytes: 240 },
    ];
    for file in files {
        println!("{}: {} bytes", file.name, file.bytes);
    }
}
