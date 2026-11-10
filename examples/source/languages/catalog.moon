-- Print labels for an in-memory file catalog.
class Entry
  new: (@name, @bytes) =>

  label: => "#{@name}: #{@bytes} bytes"

files = {
  Entry "notes.md", 120
  Entry "letter.eml", 240
}

total = 0
for entry in *files
  print entry\label!
  total += entry.bytes

print "total: #{total} bytes"
