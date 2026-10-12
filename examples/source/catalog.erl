-module(catalog).
-export([labels/0]).

labels() ->
    ["Preview: " ++ Name || Name <- ["notes.md", "letter.eml"]].
