# Print labels for an in-memory file catalog.
(def files
  [{:name "notes.md" :bytes 120}
   {:name "letter.eml" :bytes 240}])

(defn label [{:name name :bytes bytes}]
  (string/format "%s: %d bytes" name bytes))

(each entry files
  (print (label entry)))

(printf "total: %d bytes" (sum (map |($ :bytes) files)))
