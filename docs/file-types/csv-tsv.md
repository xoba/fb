# CSV and TSV

fb shows delimited text as a table you can scroll and query with SQL.

**Extensions:** `.csv` `.tsv`

## What you see

A table with bordered cells, a frozen header, and row and column numbers along its edges, tinted
pale blue to set them apart from your data. These stay visible as you scroll, so you keep your
bearings in a big table. The first row is treated as the header.

Parsing is tolerant of uneven rows and stray quotes.

### Long cells and links

Cells longer than 200 characters are cut short. If the content is plain text, the cut ends in a
**… more** link to a page with the complete cell, and its back link returns to that spot in the
table. A cell containing a single `http://` or `https://` URL becomes a link, opening outside fb
so the table keeps its place.

### Querying with SQL

The query box runs real SQLite, with the file loaded into memory as a table named `t`:

```sql
select city, sum(amount) from t group by city order by 2 desc
```

Columns whose cells all parse as numbers are typed as numbers, so `sum()` and `order by` behave
as you would expect. The box accepts one read-only statement at a time: `SELECT`, `WITH`, or
`EXPLAIN`. Queries never change your file.

## Options

| Option | Effect |
|--------|--------|
| `#cell=2,3` | Scrolls to row 2, column 3 and flashes it. See [Linking](../linking.md#links-to-table-cells) |
| `#L12` | Shows line 12 of the file's text, with a **rendered** link back. See [Linking](../linking.md#links-to-lines) |
| `?raw=1` | The original file, as does the **raw** link |

## Limits and fallbacks

Files over 2 MiB, or that fail to parse, show as plain text. Tables show at most 2,000 rows, and a
note tells you how many more there are. Query results have the same limit.

## Try it

- [library.csv](../../examples/formats/library.csv)
- [library.tsv](../../examples/formats/library.tsv)

[All file types](index.md) · [fb Help](../index.md)
