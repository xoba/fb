// Bytes written while saving an in-memory file catalog:
// notes.md (120 bytes), then letter.eml (240 bytes).
model Catalog
  parameter Integer notesBytes = 120;
  parameter Integer letterBytes = 240;
  parameter Real rate = 1000;
  Real written(start = 0, fixed = true);
equation
  // Writing stops once both files are on disk.
  der(written) = if written < notesBytes + letterBytes then rate else 0;
  annotation(experiment(StopTime = 1));
end Catalog;
