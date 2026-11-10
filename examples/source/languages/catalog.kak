# Highlight the entries of an example file catalog.
hook global BufCreate .*\.catalog %{
    set-option buffer filetype catalog
}

add-highlighter shared/catalog regions
add-highlighter shared/catalog/entry default-region regex '^(\S+): (\d+) bytes$' 1:keyword 2:value
add-highlighter shared/catalog/comment region '#' '$' fill comment

hook global WinSetOption filetype=catalog %{
    add-highlighter window/catalog ref catalog
}

define-command catalog-total -docstring 'show the catalog size' %{
    echo "360 bytes in 2 files"
}
