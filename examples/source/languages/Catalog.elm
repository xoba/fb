module Catalog exposing (main)

import Html exposing (Html, li, text, ul)


-- One file in an in-memory catalog.
type alias Entry =
    { name : String
    , bytes : Int
    }


files : List Entry
files =
    [ { name = "notes.md", bytes = 120 }
    , { name = "letter.eml", bytes = 240 }
    ]


label : Entry -> String
label entry =
    entry.name ++ ": " ++ String.fromInt entry.bytes ++ " bytes"


main : Html msg
main =
    ul [] (List.map (\entry -> li [] [ text (label entry) ]) files)
