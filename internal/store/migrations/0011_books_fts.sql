-- Full-text search over public books. LIKE-based search ranks every
-- partial match equally and scans every row, which hurts for searches
-- against long descriptions. FTS5 with bm25 ranking sorts by relevance
-- and keeps query time independent of corpus size.
--
-- `owner_name` mirrors users.display_name only when the upload isn't
-- anonymous. Owner-name lookup via JOIN inside the trigger keeps that
-- rule in one place — callers don't need to know about it.

CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
  book_id UNINDEXED,
  title,
  author,
  description,
  owner_name,
  tokenize = "unicode61 remove_diacritics 2"
);

-- Backfill on first run. Subsequent migrations are idempotent because
-- INSERT OR IGNORE skips rows already present (book_id is UNINDEXED
-- but we use it as a logical key — duplicates would just inflate
-- the index, not break anything, so a one-time backfill is fine).
INSERT INTO books_fts (book_id, title, author, description, owner_name)
SELECT
  b.id,
  COALESCE(b.title, ''),
  COALESCE(b.author, ''),
  COALESCE(b.description, ''),
  CASE WHEN u.anonymous_owner = 0 THEN COALESCE(u.display_name, '') ELSE '' END
FROM books b
JOIN users u ON u.id = b.user_id
WHERE NOT EXISTS (SELECT 1 FROM books_fts f WHERE f.book_id = b.id);

CREATE TRIGGER IF NOT EXISTS books_fts_ai
AFTER INSERT ON books BEGIN
  INSERT INTO books_fts (book_id, title, author, description, owner_name)
  SELECT
    NEW.id,
    COALESCE(NEW.title, ''),
    COALESCE(NEW.author, ''),
    COALESCE(NEW.description, ''),
    CASE WHEN u.anonymous_owner = 0 THEN COALESCE(u.display_name, '') ELSE '' END
  FROM users u WHERE u.id = NEW.user_id;
END;

CREATE TRIGGER IF NOT EXISTS books_fts_ad
AFTER DELETE ON books BEGIN
  DELETE FROM books_fts WHERE book_id = OLD.id;
END;

CREATE TRIGGER IF NOT EXISTS books_fts_au
AFTER UPDATE OF title, author, description ON books BEGIN
  UPDATE books_fts
     SET title = COALESCE(NEW.title, ''),
         author = COALESCE(NEW.author, ''),
         description = COALESCE(NEW.description, '')
   WHERE book_id = NEW.id;
END;

-- When a user's display name or anonymity toggle changes, fan out to
-- every one of their books so the FTS index stays in sync.
CREATE TRIGGER IF NOT EXISTS books_fts_user_au
AFTER UPDATE OF display_name, anonymous_owner ON users BEGIN
  UPDATE books_fts
     SET owner_name = CASE WHEN NEW.anonymous_owner = 0 THEN COALESCE(NEW.display_name, '') ELSE '' END
   WHERE book_id IN (SELECT id FROM books WHERE user_id = NEW.id);
END;
