#!/usr/bin/env -S v run
// Print the size of each example catalog file in the current directory.
names := ['notes.md', 'letter.eml']
mut total := u64(0)
for name in names {
	if !exists(name) {
		eprintln('missing: ${name}')
		continue
	}
	size := file_size(name)
	println('${name}: ${size} bytes')
	total += size
}
println('total: ${total} bytes')
