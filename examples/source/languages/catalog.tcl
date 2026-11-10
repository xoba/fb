# Print a label for each entry of a tiny in-memory catalog.
set files [dict create notes.md 120 letter.eml 240]

proc label {name bytes} {
    return [format "%s: %d bytes" $name $bytes]
}

set total 0
dict for {name bytes} $files {
    puts [label $name $bytes]
    incr total $bytes
}
puts "total: $total bytes"
