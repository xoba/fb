/* Print labels for an in-memory file catalog. */
names.1 = 'notes.md'
bytes.1 = 120
names.2 = 'letter.eml'
bytes.2 = 240
names.0 = 2

total = 0
do i = 1 to names.0
  say label(names.i, bytes.i)
  total = total + bytes.i
end
say 'total:' total 'bytes'
exit 0

label: procedure
  parse arg name, size
  return name || ':' size 'bytes'
