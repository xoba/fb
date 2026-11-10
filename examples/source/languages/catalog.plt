# Plot the size of each file listed in catalog.dat, one "name bytes" per line:
#   notes.md 120
#   letter.eml 240
set title "Catalog sizes"
set ylabel "bytes"
set style fill solid 0.6
set boxwidth 0.5
set yrange [0:*]
unset key

limit = 200
color(b) = b > limit ? 0xcc6677 : 0x4477aa
plot "catalog.dat" using 0:2:(color(column(2))):xtic(1) with boxes lc rgb variable
