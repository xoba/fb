"Print a label for each entry of a tiny in-memory catalog."
| catalog total |
catalog := Dictionary new.
catalog at: 'notes.md' put: 120.
catalog at: 'letter.eml' put: 240.
catalog keysAndValuesDo: [:name :bytes |
    Transcript
        show: name;
        show: ': ';
        show: bytes printString;
        show: ' bytes';
        cr].
total := catalog inject: 0 into: [:sum :each | sum + each].
Transcript show: 'total: ', total printString, ' bytes'; cr.
