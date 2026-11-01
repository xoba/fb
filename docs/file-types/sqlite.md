# SQLite databases

fb browses a SQLite database like a folder of tables, and lets you query it directly. It opens
databases read-only, so it cannot change them.

**Extensions:** `.sqlite` `.sqlite3` `.db`

## What you see

Opening a database lists its tables and views, each with its row and column counts and its size
on disk, including indexes. For a large database, the list appears at once and the totals follow
when they have been calculated.

Each table's page shows the total row count, the first 2,000 rows, and the table's schema with
highlighting. `NULL` appears literally, and binary values show as `(N-byte blob)`.

### Querying with SQL

The listing has a query box, and results appear as a table beneath it:

```sql
select type, count(*) from sqlite_master group by type
```

The box accepts a single read-only statement: `SELECT`, `WITH`, or `EXPLAIN`. fb checks this before
the query reaches SQLite, and opens the database read-only as well. Full-text search tables work.

## Options

| Option | Effect |
|--------|--------|
| `#cell=2,3` | On a table: scrolls to row 2, column 3 and flashes it |
| `?raw=1` | On the database: the original file. On a table: that table as CSV |

Databases are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Databases on disk open in place with no size limit, because SQLite reads only what a query needs.
A database inside an archive is first copied to a temporary file, with a limit of 100 MiB.

A `.db` file that is not really a SQLite database falls back to the original file.

## Try it

- [library.db](../../examples/formats/library.db)
- [library.sqlite](../../examples/formats/library.sqlite)
- [library.sqlite3](../../examples/formats/library.sqlite3)

[All file types](index.md) · [fb Help](../index.md)
