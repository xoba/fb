// Print labels for an in-memory file catalog.
type Entry = { Name: string; Bytes: int }

let files =
    [ { Name = "notes.md"; Bytes = 120 }
      { Name = "letter.eml"; Bytes = 240 } ]

let label entry = sprintf "%s: %d bytes" entry.Name entry.Bytes

for entry in files do
    printfn "%s" (label entry)

printfn "total: %d bytes" (files |> List.sumBy (fun e -> e.Bytes))
