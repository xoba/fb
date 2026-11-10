% Print a label for each entry of a tiny in-memory catalog.
type Entry :
    record
        name : string
        bytes : int
    end record

var files : array 1 .. 2 of Entry
files (1).name := "notes.md"
files (1).bytes := 120
files (2).name := "letter.eml"
files (2).bytes := 240

var total : int := 0
for i : 1 .. upper (files)
    put files (i).name, ": ", files (i).bytes, " bytes"
    total += files (i).bytes
end for
put "total: ", total, " bytes"
