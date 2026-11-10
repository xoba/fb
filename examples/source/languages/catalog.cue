// Schema and data for an in-memory file catalog.
package catalog

#Entry: {
	name:  string & =~"^[a-z]+\\.[a-z]+$"
	bytes: int & >0
}

files: [...#Entry] & [
	{name: "notes.md", bytes: 120},
	{name: "letter.eml", bytes: 240},
]

total: files[0].bytes + files[1].bytes
