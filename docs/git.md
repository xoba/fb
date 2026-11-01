# Git awareness

Inside a Git working tree, listings show what Git knows about each file. fb only reads:
it runs Git with `--no-optional-locks`, so browsing never changes a repository.

## Status badges

Files and folders carry small colored letters after their names. Hover over any badge for
an explanation in words.

| Badge | Meaning |
|-------|---------|
| Amber `M` | Modified |
| Green letter, such as `A` or `M` | Staged: a new file, a modification, and so on |
| Gray `?` | Untracked |
| Red `!` | Merge conflict |
| Muted `i` | Ignored |
| A dot on a folder | Contains changes. Hovering counts them: `2 modified, 1 untracked within` |

A file deleted from `HEAD` and missing from disk keeps a struck-through row without a
link, as does a missing tracked folder, so deletions stay visible until you commit them.

## The git line

The top folder of a working tree adds a muted line under the summary. It gives the
branch, how it compares with its upstream as of your last fetch (`ahead 2 of
origin/main`), and counts of changed files, with anything needing action in amber. When
nothing does, it reads `clean · in sync`.

Deeper folders show the same line with just the counts for that folder, and only when
something has changed there.

## Repository folders

A folder that is itself a repository, whether a `.git` folder or a bare repository, gets
a section at the bottom of its listing. It shows where `HEAD` points, counts of commits,
branches, and tags, the size of the object store, remotes, and the eight most recent
commits.

## When annotations are missing

Git features need the `git` command, which comes with Apple's developer tools. Without
it, listings appear normally with no annotations, and nothing else is affected.

fb gives Git 3 seconds for each listing. In a very large repository it may run out of time,
and the listing then appears without annotations. Files inside archives never have them,
because they are not on disk.

## Related pages

- [Directory listings](directory-listings.md)
- [Back to fb Help](index.md)
