' Print a label for each entry of a tiny in-memory catalog.
Imports System.Collections.Generic

Module Catalog
    Structure Entry
        Public Name As String
        Public Bytes As Integer
    End Structure

    Sub Main()
        Dim files As New List(Of Entry) From {
            New Entry With {.Name = "notes.md", .Bytes = 120},
            New Entry With {.Name = "letter.eml", .Bytes = 240}
        }
        Dim total As Integer = 0
        For Each file In files
            Console.WriteLine("{0}: {1} bytes", file.Name, file.Bytes)
            total += file.Bytes
        Next
        Console.WriteLine("total: " & total & " bytes")
    End Sub
End Module
