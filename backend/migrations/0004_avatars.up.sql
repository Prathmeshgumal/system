-- Part 2.2 — profile pictures. The file lives in object storage (S3 / RustFS);
-- the database only remembers its key. Files don't go in the database: they'd
-- bloat backups and every query that touches users.
ALTER TABLE users ADD COLUMN avatar_key text;
