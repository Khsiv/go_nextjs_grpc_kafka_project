CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name TEXT, email TEXT, description TEXT);

CREATE TABLE IF NOT EXISTS posts(
  id SERIAL PRIMARY KEY,
  users_id INTEGER NOT NULL,
  post_text TEXT,
  CONSTRAINT fk_user FOREIGN KEY(users_id) REFERENCES users(id)
);