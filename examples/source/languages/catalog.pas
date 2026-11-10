{ Print labels for an in-memory file catalog. }
program Catalog;

{$mode objfpc}{$H+}

uses SysUtils;

type
  TEntry = record
    Name: string;
    Bytes: Integer;
  end;

const
  Files: array[0..1] of TEntry = (
    (Name: 'notes.md'; Bytes: 120),
    (Name: 'letter.eml'; Bytes: 240)
  );

var
  Entry: TEntry;
  Total: Integer = 0;
begin
  for Entry in Files do
  begin
    WriteLn(Format('%s: %d bytes', [Entry.Name, Entry.Bytes]));
    Inc(Total, Entry.Bytes);
  end;
  WriteLn('total: ', Total, ' bytes');
end.
