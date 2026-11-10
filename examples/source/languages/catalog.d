// Print labels for an in-memory file catalog.
import std.stdio;
import std.algorithm : map, sum;

struct Entry
{
    string name;
    uint bytes;
}

void main()
{
    immutable files = [Entry("notes.md", 120), Entry("letter.eml", 240)];
    foreach (entry; files)
        writefln("%s: %d bytes", entry.name, entry.bytes);
    writefln("Total: %d bytes", files.map!(e => e.bytes).sum);
}
