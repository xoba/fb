;; Print labels for an in-memory file catalog.
(setv files [{"name" "notes.md" "bytes" 120}
             {"name" "letter.eml" "bytes" 240}])

(defn label [entry]
  (.format "{}: {} bytes" (get entry "name") (get entry "bytes")))

(for [entry files]
  (print (label entry)))

(print "total:" (sum (gfor entry files (get entry "bytes"))) "bytes")
