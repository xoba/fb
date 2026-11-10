// Print labels for an in-memory file catalog.
import Toybox.Lang;
import Toybox.System;

class Entry {
    var name as String;
    var bytes as Number;

    function initialize(name as String, bytes as Number) {
        self.name = name;
        self.bytes = bytes;
    }

    function label() as String {
        return name + ": " + bytes.toString() + " bytes";
    }
}

function printCatalog() as Void {
    var files = [new Entry("notes.md", 120), new Entry("letter.eml", 240)];
    for (var i = 0; i < files.size(); i++) {
        System.println(files[i].label());
    }
}
