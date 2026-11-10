// Print labels for an in-memory file catalog.
U8 *names[2] = {"notes.md", "letter.eml"};
I64 sizes[2] = {120, 240};

U0 PrintCatalog()
{
  I64 i, total = 0;
  for (i = 0; i < 2; i++) {
    "%s: %d bytes\n", names[i], sizes[i];
    total += sizes[i];
  }
  "total: %d bytes\n", total;
}

PrintCatalog;
