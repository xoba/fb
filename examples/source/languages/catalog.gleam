//// Print labels for an in-memory file catalog.

import gleam/int
import gleam/io
import gleam/list

pub type Entry {
  Entry(name: String, bytes: Int)
}

fn label(entry: Entry) -> String {
  entry.name <> ": " <> int.to_string(entry.bytes) <> " bytes"
}

pub fn main() {
  [Entry("notes.md", 120), Entry("letter.eml", 240)]
  |> list.each(fn(entry) { io.println(label(entry)) })
}
