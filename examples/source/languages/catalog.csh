#!/bin/csh -f
# Print a label for each entry of a tiny in-memory catalog.
set names = (notes.md letter.eml)
set sizes = (120 240)
@ total = 0
@ i = 1

while ($i <= $#names)
    echo "${names[$i]}: ${sizes[$i]} bytes"
    @ total += $sizes[$i]
    @ i++
end

if ($total > 200) then
    echo "total: $total bytes (large)"
else
    echo "total: $total bytes"
endif
