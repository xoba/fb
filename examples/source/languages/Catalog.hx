// Print labels for an in-memory file catalog.
typedef Entry = {
	var name:String;
	var bytes:Int;
}

class Catalog {
	static var files:Array<Entry> = [
		{name: "notes.md", bytes: 120},
		{name: "letter.eml", bytes: 240},
	];

	static function label(entry:Entry):String {
		return '${entry.name}: ${entry.bytes} bytes';
	}

	static function main() {
		for (entry in files)
			Sys.println(label(entry));
	}
}
