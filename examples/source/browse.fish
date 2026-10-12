# Print a label for each visible entry; no files are changed.
for entry in *
    printf 'Preview: %s\n' "$entry"
end
