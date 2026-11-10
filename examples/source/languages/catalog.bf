using System;

namespace Catalog
{
	// One file in an in-memory catalog.
	struct Entry
	{
		public StringView Name;
		public int Bytes;

		public this(StringView name, int bytes)
		{
			Name = name;
			Bytes = bytes;
		}
	}

	class Program
	{
		public static int Main(String[] args)
		{
			Entry[2] files = .(.("notes.md", 120), .("letter.eml", 240));
			for (let entry in files)
				Console.WriteLine("{}: {} bytes", entry.Name, entry.Bytes);
			return 0;
		}
	}
}
