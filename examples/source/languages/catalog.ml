(* Print labels for an in-memory file catalog. *)
type entry = { name : string; bytes : int }

let files = [
  { name = "notes.md"; bytes = 120 };
  { name = "letter.eml"; bytes = 240 };
]

let label entry = Printf.sprintf "%s: %d bytes" entry.name entry.bytes

let () =
  List.iter (fun entry -> print_endline (label entry)) files;
  let total = List.fold_left (fun sum entry -> sum + entry.bytes) 0 files in
  Printf.printf "total: %d bytes\n" total
