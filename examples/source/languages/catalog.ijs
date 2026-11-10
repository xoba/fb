NB. Print labels for an in-memory file catalog.
names =: 'notes.md' ; 'letter.eml'
sizes =: 120 240

label =: 4 : 0
  (> x) , ': ' , (": y) , ' bytes'
)

echo > names label each <"0 sizes
echo 'total: ' , (": +/ sizes) , ' bytes'
