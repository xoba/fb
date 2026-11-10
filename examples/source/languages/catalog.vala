// Print a label for each entry of a tiny in-memory catalog.
public class Entry : Object {
    public string name { get; construct; }
    public int bytes { get; construct; }

    public Entry (string name, int bytes) {
        Object (name: name, bytes: bytes);
    }

    public string label () {
        return "%s: %d bytes".printf (name, bytes);
    }
}

void main () {
    Entry[] files = { new Entry ("notes.md", 120), new Entry ("letter.eml", 240) };
    int total = 0;
    foreach (var file in files) {
        print ("%s\n", file.label ());
        total += file.bytes;
    }
    print ("total: %d bytes\n", total);
}
