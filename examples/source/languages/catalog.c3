module catalog;
import std::io;

// One file in an in-memory catalog.
struct Entry
{
	String name;
	uint bytes;
}

fn void main()
{
	Entry[2] files = {
		{ "notes.md", 120 },
		{ "letter.eml", 240 },
	};
	foreach (entry : files)
	{
		io::printfn("%s: %d bytes", entry.name, entry.bytes);
	}
}
