package {
    import flash.display.Sprite;

    // Print labels for an in-memory file catalog.
    public class Catalog extends Sprite {
        private var files:Array = [
            {name: "notes.md", bytes: 120},
            {name: "letter.eml", bytes: 240}
        ];

        public function Catalog() {
            for each (var entry:Object in files) {
                trace(entry.name + ": " + entry.bytes + " bytes");
            }
        }
    }
}
