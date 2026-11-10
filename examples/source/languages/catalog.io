// Print labels for an in-memory file catalog.
Entry := Object clone do(
    name ::= nil
    bytes ::= 0
    label := method(name .. ": " .. bytes .. " bytes")
)

files := list(
    Entry clone setName("notes.md") setBytes(120),
    Entry clone setName("letter.eml") setBytes(240)
)

files foreach(entry, entry label println)
("total: " .. files map(bytes) sum .. " bytes") println
