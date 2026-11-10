// Describe an in-memory file catalog and label each entry.
local entry(name, bytes) = { name: name, bytes: bytes };
local files = [entry('notes.md', 120), entry('letter.eml', 240)];

{
  files: files,
  labels: ['%s: %d bytes' % [f.name, f.bytes] for f in files],
  total: std.foldl(function(sum, f) sum + f.bytes, files, 0),
  readOnly: true,
}
