local files = {
    {name = "notes.md", bytes = 120},
    {name = "letter.eml", bytes = 240},
}

for _, file in ipairs(files) do
    print(string.format("%s: %d bytes", file.name, file.bytes))
end
