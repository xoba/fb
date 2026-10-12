files = ["notes.md", "letter.eml"]
sizes = [120, 240]

for (name, bytes) in zip(files, sizes)
    println("$name: $bytes bytes")
end
println("Total: ", sum(sizes))
