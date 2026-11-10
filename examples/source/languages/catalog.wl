(* Print labels for an in-memory file catalog. *)
files = {
  <|"name" -> "notes.md", "bytes" -> 120|>,
  <|"name" -> "letter.eml", "bytes" -> 240|>
};

label[entry_Association] :=
  entry["name"] <> ": " <> ToString[entry["bytes"]] <> " bytes"

Scan[Print[label[#]] &, files]
Print["total: ", Total[Lookup[files, "bytes"]], " bytes"]
