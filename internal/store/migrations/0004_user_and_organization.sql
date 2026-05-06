-- Account: display name (used for public attribution + topbar)
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

-- Reading time: cached estimate so we don't re-derive on every page load
ALTER TABLE books ADD COLUMN reading_minutes INTEGER NOT NULL DEFAULT 0;

-- Bookmarks: lightweight "save my place at X" separate from highlights
CREATE TABLE IF NOT EXISTS bookmarks (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  book_id TEXT NOT NULL,
  locator TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_bookmarks_user_book_created
  ON bookmarks (user_id, book_id, created_at DESC);

-- Tags: per-user labels attached to books
CREATE TABLE IF NOT EXISTS book_tags (
  user_id TEXT NOT NULL,
  book_id TEXT NOT NULL,
  tag TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (user_id, book_id, tag),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_book_tags_user_tag ON book_tags (user_id, tag);
