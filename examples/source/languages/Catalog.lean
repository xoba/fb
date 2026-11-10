/-! # A tiny file catalog

  Model a few example files, label them, and prove a fact about the labels.
-/

structure Entry where
  name  : String
  bytes : Nat
deriving Repr

def catalog : List Entry :=
  [⟨"notes.md", 120⟩, ⟨"letter.eml", 240⟩]

/-- A label such as `notes.md: 120 bytes`. -/
def Entry.label (e : Entry) : String :=
  s!"{e.name}: {e.bytes} bytes"

def totalBytes (entries : List Entry) : Nat :=
  entries.foldl (fun acc e => acc + e.bytes) 0

#eval catalog.map Entry.label   -- ["notes.md: 120 bytes", "letter.eml: 240 bytes"]
#eval totalBytes catalog        -- 360

theorem totalBytes_nil : totalBytes [] = 0 := rfl

theorem catalog_total : totalBytes catalog = 360 := by
  decide
