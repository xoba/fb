with Ada.Text_IO; use Ada.Text_IO;
procedure Catalog is
   Names : constant array (1 .. 2) of String (1 .. 9) :=
     ("notes.md ", "letter.md");
begin
   for Name of Names loop
      Put_Line (Name);
   end loop;
end Catalog;
