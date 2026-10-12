files = [
  { name: "notes.md", bytes: 120 },
  { name: "letter.eml", bytes: 240 }
]

files.each { |file| puts "#{file[:name]}: #{file[:bytes]} bytes" }
