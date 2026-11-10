// Print labels for an in-memory file catalog.
struct Entry {
  name : String
  bytes : Int
}

fn Entry::label(self : Entry) -> String {
  "\{self.name}: \{self.bytes} bytes"
}

fn main {
  let files : Array[Entry] = [
    { name: "notes.md", bytes: 120 },
    { name: "letter.eml", bytes: 240 },
  ]
  let mut total = 0
  for entry in files {
    println(entry.label())
    total += entry.bytes
  }
  println("total: \{total} bytes")
}
