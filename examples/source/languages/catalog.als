// A file catalog in which every entry has one name and a positive size.
module catalog

sig Name {}

sig Entry {
  name: one Name,
  bytes: one Int
}

fact PositiveSizes {
  all e: Entry | e.bytes > 0
}

fact UniqueNames {
  all disj a, b: Entry | a.name != b.name
}

run {} for 3 but exactly 2 Entry
