// Shows a label for each file in an in-memory catalog.
codeunit 50100 "File Catalog"
{
    trigger OnRun()
    var
        Names: List of [Text];
        Sizes: List of [Integer];
        Index: Integer;
    begin
        Names.Add('notes.md');
        Sizes.Add(120);
        Names.Add('letter.eml');
        Sizes.Add(240);
        for Index := 1 to Names.Count() do
            Message('%1: %2 bytes', Names.Get(Index), Sizes.Get(Index));
    end;
}
