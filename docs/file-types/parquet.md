# Parquet

Parquet is a columnar data format common in data engineering. fb shows Parquet files in the same
table view as [CSV](csv-tsv.md), with the same query box, and needs no other tools.

**Extensions:** `.parquet`

## What you see

A table with a frozen header and row and column numbers. Columns follow the file's schema, and
`NULL` values appear empty. The rules for long cells and links are the same as for
[CSV](csv-tsv.md#long-cells-and-links).

### Querying with SQL

The query box loads the rows fb has read into an in-memory SQLite table named `t`. See
[Querying with SQL](csv-tsv.md#querying-with-sql).

## Options

| Option | Effect |
|--------|--------|
| `#cell=2,3` | Scrolls to row 2, column 3 and flashes it |
| `?raw=1` | The original file |

Parquet files are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

| Limit | Value |
|-------|-------|
| File size | 20 MiB |
| Rows read | The first 100,000. The summary tells you when this applies |
| Rows displayed | 2,000 |

Files that are too large or fail to decode fall back to the original file.

## Try it

[library.parquet](../../examples/formats/library.parquet)

[All file types](index.md) · [fb Help](../index.md)
