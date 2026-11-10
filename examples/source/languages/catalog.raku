# Print labels for an in-memory file catalog.
class Entry {
    has Str $.name is required;
    has Int $.bytes is required;

    method label(--> Str) { "$!name: $!bytes bytes" }
}

my @files =
    Entry.new(name => 'notes.md',   bytes => 120),
    Entry.new(name => 'letter.eml', bytes => 240);

.label.say for @files;
say "total: { @files.map(*.bytes).sum } bytes";
