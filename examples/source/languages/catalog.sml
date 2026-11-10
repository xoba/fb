(* Print a label for each entry of a tiny in-memory catalog. *)
type entry = {name : string, bytes : int}

val files : entry list =
  [ {name = "notes.md", bytes = 120}
  , {name = "letter.eml", bytes = 240} ]

fun label ({name, bytes} : entry) =
  name ^ ": " ^ Int.toString bytes ^ " bytes"

val total = foldl (fn ({bytes, ...} : entry, sum) => sum + bytes) 0 files

val () = List.app (fn file => print (label file ^ "\n")) files
val () = print ("total: " ^ Int.toString total ^ " bytes\n")
