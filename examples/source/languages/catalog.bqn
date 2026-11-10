# Print labels for an in-memory file catalog.
names ← ⟨"notes.md", "letter.eml"⟩
bytes ← ⟨120, 240⟩
Label ← {𝕨∾": "∾(•Fmt 𝕩)∾" bytes"}
•Out¨ names Label¨ bytes
•Out "Total: "∾(•Fmt +´bytes)∾" bytes"
