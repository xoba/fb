# Display sample records without modifying the filesystem.
$catalogEntries = @(
    [PSCustomObject]@{ Name = 'notes.md'; Bytes = 120 }
    [PSCustomObject]@{ Name = 'letter.eml'; Bytes = 240 }
)
$catalogEntries | Format-Table -AutoSize
