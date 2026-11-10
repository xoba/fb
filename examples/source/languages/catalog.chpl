// Print labels for an in-memory file catalog.
record Entry {
  var name: string;
  var bytes: int;
}

const files = [new Entry("notes.md", 120), new Entry("letter.eml", 240)];

for entry in files do
  writeln(entry.name, ": ", entry.bytes, " bytes");

const total = + reduce [entry in files] entry.bytes;
writeln("Total: ", total, " bytes");
