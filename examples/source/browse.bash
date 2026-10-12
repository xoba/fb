# A read-only listing, intended to be viewed as source.
folder=${1:-.}
printf 'Files in %s\n' "$folder"
for entry in "$folder"/*; do
    [[ -e "$entry" ]] && printf '%s\n' "${entry##*/}"
done
