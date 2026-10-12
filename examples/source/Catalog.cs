using System;

public static class Catalog
{
    public static void Main()
    {
        foreach (var name in new[] { "notes.md", "letter.eml" })
            Console.WriteLine(name);
    }
}
