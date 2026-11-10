--!strict
-- Print labels for an in-memory file catalog.
type Entry = {
	name: string,
	bytes: number,
}

local files: { Entry } = {
	{ name = "notes.md", bytes = 120 },
	{ name = "letter.eml", bytes = 240 },
}

local function label(entry: Entry): string
	return `{entry.name}: {entry.bytes} bytes`
end

for _, entry in files do
	print(label(entry))
end
