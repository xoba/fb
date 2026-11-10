' Print labels for an in-memory file catalog.
TYPE Entry
    FileName AS STRING * 12
    Bytes AS INTEGER
END TYPE

DIM files(1 TO 2) AS Entry
files(1).FileName = "notes.md": files(1).Bytes = 120
files(2).FileName = "letter.eml": files(2).Bytes = 240

total = 0
FOR i = 1 TO 2
    PRINT RTRIM$(files(i).FileName); ":"; files(i).Bytes; "bytes"
    total = total + files(i).Bytes
NEXT i
PRINT "total:"; total; "bytes"
END
