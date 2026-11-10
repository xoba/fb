# Print labels for an in-memory file catalog.


@fieldwise_init
struct Entry(Copyable, Movable):
    var name: String
    var bytes: Int

    fn label(self) -> String:
        return String(self.name, ": ", self.bytes, " bytes")


def main():
    var files = [Entry("notes.md", 120), Entry("letter.eml", 240)]
    var total = 0
    for entry in files:
        print(entry.label())
        total += entry.bytes
    print("total:", total, "bytes")
