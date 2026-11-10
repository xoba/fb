#lang racket/base
;; Print labels for an in-memory file catalog.

(struct entry (name bytes))

(define files
  (list (entry "notes.md" 120)
        (entry "letter.eml" 240)))

(define (label e)
  (format "~a: ~a bytes" (entry-name e) (entry-bytes e)))

(for ([e (in-list files)])
  (displayln (label e)))

(printf "total: ~a bytes\n"
        (for/sum ([e (in-list files)]) (entry-bytes e)))
