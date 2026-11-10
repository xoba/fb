;; Print labels for an in-memory file catalog.
(define-record-type entry
  (make-entry name bytes)
  entry?
  (name entry-name)
  (bytes entry-bytes))

(define files
  (list (make-entry "notes.md" 120)
        (make-entry "letter.eml" 240)))

(define (label e)
  (string-append (entry-name e) ": "
                 (number->string (entry-bytes e)) " bytes"))

(for-each (lambda (e) (display (label e)) (newline)) files)

(display "total: ")
(display (apply + (map entry-bytes files)))
(display " bytes")
(newline)
