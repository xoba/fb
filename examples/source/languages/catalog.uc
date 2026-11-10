// Print a label for each entry of a tiny in-memory catalog.
let files = [
	{ name: "notes.md", bytes: 120 },
	{ name: "letter.eml", bytes: 240 }
];

function label(file) {
	return sprintf("%s: %d bytes", file.name, file.bytes);
}

let total = 0;
for (let file in files) {
	print(label(file), "\n");
	total += file.bytes;
}
printf("total: %d bytes\n", total);
