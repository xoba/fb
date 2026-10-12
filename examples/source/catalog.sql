-- An illustrative in-memory catalog schema and query.
CREATE TABLE files (
    name TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    bytes INTEGER NOT NULL CHECK (bytes >= 0)
);

INSERT INTO files VALUES
    ('notes.md', 'document', 120),
    ('letter.eml', 'message', 240);

SELECT kind, COUNT(*) AS file_count, SUM(bytes) AS total_bytes
FROM files
GROUP BY kind
ORDER BY kind;
