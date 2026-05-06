CREATE TABLE IF NOT EXISTS wishlist (
    user_id TEXT NOT NULL,
    book_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (user_id, book_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_wishlist_user ON wishlist(user_id, created_at DESC);
