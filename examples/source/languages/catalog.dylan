Module: catalog
Synopsis: Print labels for an in-memory file catalog.

define class <entry> (<object>)
  constant slot entry-name :: <string>, required-init-keyword: name:;
  constant slot entry-bytes :: <integer>, required-init-keyword: bytes:;
end class <entry>;

define constant $files
  = vector(make(<entry>, name: "notes.md", bytes: 120),
           make(<entry>, name: "letter.eml", bytes: 240));

// Print one label per entry.
define function main () => ()
  for (entry in $files)
    format-out("%s: %d bytes\n", entry.entry-name, entry.entry-bytes);
  end for;
end function main;

main();
