# Allow reading catalog entries that are small text files.
package catalog.access

import rego.v1

default allow := false

text_extensions := {"md", "eml", "txt"}

allow if {
	input.method == "GET"
	some ext in text_extensions
	endswith(input.file.name, concat("", [".", ext]))
	input.file.bytes <= 1024
}

labels contains label if {
	some file in data.catalog.files
	label := sprintf("%s: %d bytes", [file.name, file.bytes])
}
