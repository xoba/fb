#!/usr/bin/sed -f
# Turn "name bytes" catalog lines into labels such as "notes.md: 120 bytes".
/^#/d
/^[[:space:]]*$/d
s/^\([^ ]*\)  *\([0-9][0-9]*\)$/\1: \2 bytes/
# Mark entries of 200 bytes or more as large.
/: [2-9][0-9][0-9] bytes$/s/$/ (large)/
