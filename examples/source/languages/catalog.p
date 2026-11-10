/* Print labels for an in-memory file catalog. */
DEFINE TEMP-TABLE ttEntry NO-UNDO
    FIELD Name  AS CHARACTER
    FIELD Bytes AS INTEGER.

DEFINE VARIABLE iTotal AS INTEGER NO-UNDO.

CREATE ttEntry.
ASSIGN ttEntry.Name = "notes.md" ttEntry.Bytes = 120.
CREATE ttEntry.
ASSIGN ttEntry.Name = "letter.eml" ttEntry.Bytes = 240.

FOR EACH ttEntry BY ttEntry.Name:
    MESSAGE ttEntry.Name + ": " + STRING(ttEntry.Bytes) + " bytes".
    iTotal = iTotal + ttEntry.Bytes.
END.

MESSAGE "total:" iTotal "bytes".
