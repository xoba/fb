# cython: language_level=3
"""Print labels for an in-memory file catalog."""

cdef struct Entry:
    const char *name
    int bytes

cdef Entry files[2]
files[0] = Entry(b"notes.md", 120)
files[1] = Entry(b"letter.eml", 240)

cpdef int total():
    cdef int i, n = 0
    for i in range(2):
        n += files[i].bytes
    return n

for i in range(2):
    print(f"{files[i].name.decode()}: {files[i].bytes} bytes")
print(f"Total: {total()} bytes")
