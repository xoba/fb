// Print labels for an in-memory file catalog.
use fmt;

type entry = struct {
	name: str,
	bytes: size,
};

export fn main() void = {
	const files: [_]entry = [
		entry { name = "notes.md", bytes = 120 },
		entry { name = "letter.eml", bytes = 240 },
	];
	for (let i = 0z; i < len(files); i += 1) {
		fmt::printfln("{}: {} bytes", files[i].name, files[i].bytes)!;
	};
};
