-- Tokens de solo lectura y de todos los proyectos (contrato §8.1), para el
-- servidor MCP. project_id deja de ser NOT NULL: NULL es "todos los
-- proyectos", que solo existe con read_only = 1. SQLite no quita un NOT NULL,
-- así que la tabla se reconstruye. AUTOINCREMENT: el id de un token va en la
-- auditoría (actor_id) y en DELETE /tokens/{id}, y no se reutiliza.

CREATE TABLE api_tokens_new (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL,
  token_hash   TEXT    NOT NULL UNIQUE,
  project_id   INTEGER REFERENCES projects(id) ON DELETE CASCADE,
  stage_id     INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  read_only    INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1)),
  all_projects INTEGER NOT NULL DEFAULT 0 CHECK (all_projects IN (0, 1)),
  created_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER,
  expires_at   INTEGER,
  -- Sin proyecto si y solo si es de todos los proyectos.
  CHECK ((all_projects = 1) = (project_id IS NULL)),
  -- Todos los proyectos: sin stage y siempre de solo lectura (contrato §8.1).
  CHECK (all_projects = 0 OR (read_only = 1 AND stage_id IS NULL))
);

INSERT INTO api_tokens_new (id, name, token_hash, project_id, stage_id, created_by, created_at, last_used_at, expires_at)
  SELECT id, name, token_hash, project_id, stage_id, created_by, created_at, last_used_at, expires_at FROM api_tokens;

-- La secuencia de AUTOINCREMENT sigue: si el token más nuevo se borró, su id
-- no vuelve. El INSERT de arriba solo deja en api_tokens_new el id más alto
-- que copió (o ninguno, sin filas).
INSERT INTO sqlite_sequence (name, seq)
  SELECT 'api_tokens_new', seq FROM sqlite_sequence WHERE name = 'api_tokens'
   AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'api_tokens_new');
UPDATE sqlite_sequence SET seq = max(seq, (SELECT seq FROM sqlite_sequence WHERE name = 'api_tokens'))
 WHERE name = 'api_tokens_new' AND EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'api_tokens');

DROP TABLE api_tokens;
-- RENAME también renombra la fila de sqlite_sequence.
ALTER TABLE api_tokens_new RENAME TO api_tokens;
