// A tiny catalog of example files with a verified total size.
type Entry is { int id, int bytes } where bytes >= 0

function total(Entry[] files) -> (int sum)
ensures sum >= 0:
    int s = 0
    int i = 0
    while i < |files| where i >= 0 && s >= 0:
        s = s + files[i].bytes
        i = i + 1
    return s

public export method test():
    // notes.md is entry 1 and letter.eml is entry 2.
    Entry[] files = [{id: 1, bytes: 120}, {id: 2, bytes: 240}]
    assert total(files) >= 0
