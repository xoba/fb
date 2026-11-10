import ballerina/io;

// One file in an in-memory catalog.
type Entry record {|
    string name;
    int bytes;
|};

public function main() {
    Entry[] files = [
        {name: "notes.md", bytes: 120},
        {name: "letter.eml", bytes: 240}
    ];
    foreach Entry entry in files {
        io:println(entry.name, ": ", entry.bytes, " bytes");
    }
}
