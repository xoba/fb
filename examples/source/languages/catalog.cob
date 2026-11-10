      * Print labels for an in-memory file catalog.
       IDENTIFICATION DIVISION.
       PROGRAM-ID. CATALOG.

       DATA DIVISION.
       WORKING-STORAGE SECTION.
       01 FILE-TABLE.
          05 FILE-ENTRY OCCURS 2 TIMES INDEXED BY IDX.
             10 FILE-NAME  PIC X(12).
             10 FILE-BYTES PIC 9(4).
       01 BYTES-OUT        PIC Z(3)9.

       PROCEDURE DIVISION.
           MOVE "notes.md"   TO FILE-NAME(1)
           MOVE 120          TO FILE-BYTES(1)
           MOVE "letter.eml" TO FILE-NAME(2)
           MOVE 240          TO FILE-BYTES(2)
           PERFORM VARYING IDX FROM 1 BY 1 UNTIL IDX > 2
               MOVE FILE-BYTES(IDX) TO BYTES-OUT
               DISPLAY FUNCTION TRIM(FILE-NAME(IDX)) ": "
                   FUNCTION TRIM(BYTES-OUT) " bytes"
           END-PERFORM
           STOP RUN.
