# Pure data: evaluating this expression does not fetch or build anything.
let
  files = [ "notes.md" "letter.eml" ];
in {
  title = "Example folder";
  inherit files;
  count = builtins.length files;
}
