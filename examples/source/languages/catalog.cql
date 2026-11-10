-- A small table of example files and their sizes.
CREATE KEYSPACE IF NOT EXISTS catalog
  WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

CREATE TABLE IF NOT EXISTS catalog.files (
  name text PRIMARY KEY,
  bytes int,
  added timestamp
);

INSERT INTO catalog.files (name, bytes, added) VALUES ('notes.md', 120, toTimestamp(now()));
INSERT INTO catalog.files (name, bytes, added) VALUES ('letter.eml', 240, toTimestamp(now()));

SELECT name, bytes FROM catalog.files WHERE name = 'notes.md';
