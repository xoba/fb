// Catalog service: list example files and their sizes.
namespace go catalog
namespace py catalog

const i32 MAX_BYTES = 4096

enum Kind {
  NOTE = 1,
  MAIL = 2,
}

struct Entry {
  1: required string name,
  2: required i64 bytes,
  3: optional Kind kind = Kind.NOTE,
}

exception NotFound {
  1: string name,
}

service Catalog {
  list<Entry> listEntries(),
  Entry lookup(1: string name) throws (1: NotFound missing),
}
