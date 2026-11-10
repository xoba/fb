" Print a label for each entry of a tiny in-memory catalog.
let s:files = [
      \ {'name': 'notes.md', 'bytes': 120},
      \ {'name': 'letter.eml', 'bytes': 240},
      \ ]

function! s:Label(file) abort
  return printf('%s: %d bytes', a:file.name, a:file.bytes)
endfunction

function! CatalogList() abort
  let l:total = 0
  for l:file in s:files
    echo s:Label(l:file)
    let l:total += l:file.bytes
  endfor
  echo 'total: ' . l:total . ' bytes'
endfunction

command! CatalogList call CatalogList()
