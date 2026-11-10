@0xd3b5c8a1f2e4b6c7;
# One file in an in-memory catalog.

struct Entry {
  name @0 :Text;
  bytes @1 :UInt32;
  kind @2 :Kind;

  enum Kind {
    note @0;
    letter @1;
  }
}

struct Catalog {
  entries @0 :List(Entry);
}
