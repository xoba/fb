* Print labels for an in-memory file catalog.
REPORT zfb_catalog.

TYPES: BEGIN OF ty_entry,
         name  TYPE string,
         bytes TYPE i,
       END OF ty_entry.

DATA: lt_files TYPE STANDARD TABLE OF ty_entry WITH EMPTY KEY,
      ls_file  TYPE ty_entry.

ls_file-name = 'notes.md'.
ls_file-bytes = 120.
APPEND ls_file TO lt_files.
ls_file-name = 'letter.eml'.
ls_file-bytes = 240.
APPEND ls_file TO lt_files.

LOOP AT lt_files INTO ls_file.
  WRITE: / ls_file-name, ls_file-bytes, 'bytes'.
ENDLOOP.
