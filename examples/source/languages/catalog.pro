% Print labels for an in-memory file catalog.
entry('notes.md', 120).
entry('letter.eml', 240).

label(Name, Label) :-
    entry(Name, Bytes),
    format(atom(Label), "~w: ~d bytes", [Name, Bytes]).

total(Total) :-
    findall(Bytes, entry(_, Bytes), Sizes),
    sum_list(Sizes, Total).

main :-
    forall(label(_, Label), (write(Label), nl)),
    total(Total),
    format("total: ~d bytes~n", [Total]).

:- initialization(main).
