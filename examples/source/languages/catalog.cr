# Print labels for an in-memory file catalog.
record Entry, name : String, bytes : Int32

files = [
  Entry.new("notes.md", 120),
  Entry.new("letter.eml", 240),
]

files.each do |entry|
  puts "#{entry.name}: #{entry.bytes} bytes"
end

puts "Total: #{files.sum(&.bytes)} bytes"
