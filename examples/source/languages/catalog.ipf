#pragma rtGlobals=3
#pragma TextEncoding = "UTF-8"

// Print labels for an in-memory file catalog.
Function PrintCatalog()
	Make/O/T names = {"notes.md", "letter.eml"}
	Make/O sizes = {120, 240}
	Variable i
	for (i = 0; i < numpnts(sizes); i += 1)
		printf "%s: %d bytes\r", names[i], sizes[i]
	endfor
	printf "total: %d bytes\r", sum(sizes)
End
