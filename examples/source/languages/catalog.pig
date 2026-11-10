-- Summarize an example file catalog by extension.
files = LOAD 'catalog.tsv' USING PigStorage('\t') AS (name:chararray, bytes:int);

labeled = FOREACH files GENERATE name, bytes,
    REGEX_EXTRACT(name, '\\.([a-z]+)$', 1) AS ext;

by_ext = GROUP labeled BY ext;
totals = FOREACH by_ext GENERATE group AS ext,
    COUNT(labeled) AS files, SUM(labeled.bytes) AS bytes;

sorted = ORDER totals BY bytes DESC;
DUMP sorted;
