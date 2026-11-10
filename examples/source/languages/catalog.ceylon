"One file in an in-memory catalog."
class Entry(shared String name, shared Integer bytes) {}

// Print a label for each entry, then the total size.
shared void run() {
    value files = [
        Entry("notes.md", 120),
        Entry("letter.eml", 240)
    ];
    for (entry in files) {
        print("``entry.name``: ``entry.bytes`` bytes");
    }
    print("Total: ``sum(files*.bytes)`` bytes");
}
