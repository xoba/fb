# Count comma-separated example rows, skipping the heading.
BEGIN { FS = "," }
NR > 1 { count[$2]++ }
END { for (kind in count) print kind, count[kind] }
