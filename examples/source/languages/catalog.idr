-- Print labels for an in-memory file catalog.
record Entry where
  constructor MkEntry
  name : String
  bytes : Nat

files : List Entry
files = [MkEntry "notes.md" 120, MkEntry "letter.eml" 240]

label : Entry -> String
label entry = name entry ++ ": " ++ show (bytes entry) ++ " bytes"

main : IO ()
main = traverse_ (putStrLn . label) files
