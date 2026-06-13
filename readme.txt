i want to make a localhost browser which is always on, which enables
me to browse my filesystem but automatically converting md files
to html on the fly, so i can view them better.

let's write it in the go language. but let it call out to pandoc
for the *.md files. we can follow the style of ~/desktop-stuff/html
script.

let it use func ServeFileFS(w ResponseWriter, r *Request, fsys
fs.FS, name string) from net/http package.  it will open up an fs.FS
in the directory it is started in. it will just pass all files
as-is, as it normally operates, but for *.md files, it will convert
to html on the fly and serve that instead.

let's use http://localhost:3030 as our endpoint.


