# Excel workbooks

fb browses an Excel workbook like a folder, with each sheet as a table you can read and query.

**Extensions:** `.xlsx`

## What you see

Opening a workbook lists its sheets with their row counts. Each sheet opens as a table with a
frozen header and row and column numbers, in the same view as [CSV](csv-tsv.md). Any blank rows
above a sheet's data are skipped, so the first populated row becomes the header.

fb shows values, not formatting, formulas, or charts.

### Querying across sheets

The workbook's listing has a query box with one SQLite table per sheet, so joins across sheets
work. Sheet names become identifiers, so `Q1 Sales` is queried as `Q1_Sales`:

```sql
select * from Q1_Sales join Q2_Sales using (region)
```

Columns whose cells all parse as numbers are typed as numbers. The box accepts one read-only
statement at a time: `SELECT`, `WITH`, or `EXPLAIN`.

## Options

| Option | Effect |
|--------|--------|
| `#cell=2,3` | On a sheet: scrolls to row 2, column 3 and flashes it |
| `?raw=1` | On the workbook: the original file. On a sheet: that sheet as CSV |

Workbooks are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Workbooks over 10 MiB fall back to the original file. Sheets show at most 2,000 rows. The older
`.xls` format is not supported.

## Try it

[library.xlsx](../../examples/formats/library.xlsx)

[All file types](index.md) · [fb Help](../index.md)
