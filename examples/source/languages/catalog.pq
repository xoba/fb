// Label each entry of an in-memory file catalog, largest first.
let
    Source = #table(
        type table [Name = text, Bytes = Int64.Type],
        {
            {"notes.md", 120},
            {"letter.eml", 240}
        }
    ),
    WithLabel = Table.AddColumn(Source, "Label",
        each [Name] & ": " & Text.From([Bytes]) & " bytes", type text),
    Sorted = Table.Sort(WithLabel, {{"Bytes", Order.Descending}})
in
    Sorted
