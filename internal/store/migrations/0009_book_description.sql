-- Add a plain-text description to books. NOT NULL DEFAULT '' so the
-- column appears empty (not NULL) for every existing row, which keeps the
-- SELECT/Scan logic simple — we treat "" as "no description" everywhere.
ALTER TABLE books ADD COLUMN description TEXT NOT NULL DEFAULT '';
