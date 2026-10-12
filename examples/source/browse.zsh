# Null-glob keeps an empty folder from producing a literal asterisk.
setopt NULL_GLOB
for entry in *; do
    printf 'Preview: %s\n' "$entry"
done
