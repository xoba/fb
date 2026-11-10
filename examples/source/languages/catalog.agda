-- An in-memory file catalog with sizes in bytes.
module catalog where

open import Data.Nat using (ℕ; _+_)
open import Data.String using (String)
open import Data.List using (List; _∷_; []; foldr)

record Entry : Set where
  constructor entry
  field
    name  : String
    bytes : ℕ

open Entry

files : List Entry
files = entry "notes.md" 120 ∷ entry "letter.eml" 240 ∷ []

total : List Entry → ℕ
total = foldr (λ e n → bytes e + n) 0
