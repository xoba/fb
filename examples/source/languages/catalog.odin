// Print labels for an in-memory file catalog.
package catalog

import "core:fmt"

Entry :: struct {
	name:  string,
	bytes: int,
}

main :: proc() {
	files := []Entry{
		{name = "notes.md", bytes = 120},
		{name = "letter.eml", bytes = 240},
	}
	total := 0
	for entry in files {
		fmt.printf("%s: %d bytes\n", entry.name, entry.bytes)
		total += entry.bytes
	}
	fmt.println("total:", total, "bytes")
}
