(* An in-memory file catalog and its total size. *)
Require Import Coq.Strings.String Coq.Lists.List.
Import ListNotations.
Open Scope string_scope.

Record entry := mkEntry { name : string; bytes : nat }.

Definition files : list entry :=
  [mkEntry "notes.md" 120; mkEntry "letter.eml" 240].

Definition total (es : list entry) : nat :=
  fold_left (fun acc e => acc + bytes e) es 0.

Example total_files : total files = 360.
Proof. reflexivity. Qed.
