// Print labels for an in-memory file catalog.
names = ["notes.md", "letter.eml"];
sizes = [120, 240];

function label = catalog_label(name, bytes)
    label = msprintf("%s: %d bytes", name, bytes);
endfunction

for i = 1:size(names, "*")
    mprintf("%s\n", catalog_label(names(i), sizes(i)));
end
mprintf("total: %d bytes\n", sum(sizes));
