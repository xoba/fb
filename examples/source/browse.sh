# POSIX shell: display names without changing any files.
for entry in ./*; do
    [ -e "$entry" ] || continue
    printf 'Preview: %s\n' "${entry#./}"
done
