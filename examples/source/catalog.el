;;; catalog.el --- A tiny file catalog example -*- lexical-binding: t; -*-

(defun catalog-label (name)
  "Return a display label for file NAME."
  (format "Preview: %s" name))

(mapcar #'catalog-label '("notes.md" "letter.eml"))
