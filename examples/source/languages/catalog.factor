! Print labels for an in-memory file catalog.
USING: accessors arrays formatting kernel sequences ;
IN: catalog

TUPLE: entry name bytes ;
C: <entry> entry

: files ( -- seq )
    "notes.md" 120 <entry>
    "letter.eml" 240 <entry> 2array ;

: print-labels ( -- )
    files [ [ name>> ] [ bytes>> ] bi "%s: %d bytes\n" printf ] each ;

MAIN: print-labels
