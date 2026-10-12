(defparameter *files* '("notes.md" "letter.eml"))

(defun catalog-label (name)
  (format nil "Preview: ~a" name))

(dolist (name *files*)
  (format t "~a~%" (catalog-label name)))
