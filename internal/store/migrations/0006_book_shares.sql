CREATE TABLE IF NOT EXISTS book_shares (
    book_id TEXT NOT NULL,
    shared_with_user_id TEXT NOT NULL,
    shared_by_user_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (book_id, shared_with_user_id),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    FOREIGN KEY (shared_with_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (shared_by_user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_book_shares_recipient ON book_shares(shared_with_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_book_shares_book ON book_shares(book_id);
