\ Print labels for an in-memory file catalog.
: .label ( c-addr u bytes -- )
  >r type ." : " r> 0 .r ."  bytes" cr ;

: catalog ( -- )
  s" notes.md" 120 .label
  s" letter.eml" 240 .label ;

catalog
