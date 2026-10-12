files = ["notes.md", "letter.eml"]

files
|> Enum.map(&String.upcase/1)
|> Enum.each(&IO.puts/1)
