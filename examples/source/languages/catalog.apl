⍝ Print labels for an in-memory file catalog.
names ← 'notes.md' 'letter.eml'
bytes ← 120 240
Label ← {⍺,': ',(⍕⍵),' bytes'}
⎕ ← ↑names Label¨ bytes
⎕ ← 'Total: ',(⍕+/bytes),' bytes'
