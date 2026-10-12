-- Todas las marcas de tiempo son milisegundos Unix (UTC).
-- stages, apps y users usan AUTOINCREMENT porque sus ids forman parte de la
-- AAD de valores cifrados: un id no se reutiliza (§12 del spec). api_tokens
-- también, porque su id queda en la auditoría (actor_id) y en DELETE /tokens/{id}.

CREATE TABLE projects (
  id         INTEGER PRIMARY KEY,
  slug       TEXT    NOT NULL UNIQUE,
  name       TEXT    NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE stages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  slug       TEXT    NOT NULL,
  created_at INTEGER NOT NULL,
  UNIQUE (project_id, slug)
);

CREATE TABLE apps (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  stage_id          INTEGER NOT NULL REFERENCES stages(id) ON DELETE RESTRICT,
  slug              TEXT    NOT NULL,
  kind              TEXT    NOT NULL CHECK (kind IN ('service', 'compose')),
  spec              TEXT,
  compose_yaml      TEXT,
  deploy_timeout_s  INTEGER NOT NULL DEFAULT 300,
  storage_node_id   TEXT,
  config_changed_at INTEGER NOT NULL,
  last_deployed_at  INTEGER,
  created_at        INTEGER NOT NULL,
  UNIQUE (stage_id, slug),
  CHECK ((kind = 'service' AND spec IS NOT NULL AND compose_yaml IS NULL)
      OR (kind = 'compose' AND compose_yaml IS NOT NULL AND spec IS NULL))
);

-- Exactamente uno de stage_id / app_id. El valor siempre va cifrado.
CREATE TABLE env_vars (
  id         INTEGER PRIMARY KEY,
  stage_id   INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  app_id     INTEGER REFERENCES apps(id) ON DELETE CASCADE,
  key        TEXT    NOT NULL,
  value      BLOB    NOT NULL,
  secret     INTEGER NOT NULL DEFAULT 1,
  updated_at INTEGER NOT NULL,
  CHECK ((stage_id IS NULL) <> (app_id IS NULL))
);
CREATE UNIQUE INDEX env_vars_stage_key ON env_vars(stage_id, key) WHERE stage_id IS NOT NULL;
CREATE UNIQUE INDEX env_vars_app_key ON env_vars(app_id, key) WHERE app_id IS NOT NULL;

CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  totp_secret   BLOB,
  totp_pending  BLOB,
  created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at    INTEGER NOT NULL
);

-- created_by NULL = invitación de setup creada por el sistema.
CREATE TABLE invites (
  id         INTEGER PRIMARY KEY,
  token_hash TEXT    NOT NULL UNIQUE,
  created_by INTEGER REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL,
  used_at    INTEGER
);

CREATE TABLE sessions (
  id_hash      TEXT    PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mfa_pending  INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL,
  user_agent   TEXT    NOT NULL DEFAULT '',
  ip           TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL,
  token_hash   TEXT    NOT NULL UNIQUE,
  project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  stage_id     INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  created_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER,
  expires_at   INTEGER
);

CREATE TABLE audit_events (
  id         INTEGER PRIMARY KEY,
  actor_type TEXT    NOT NULL,
  actor_id   INTEGER,
  action     TEXT    NOT NULL,
  target     TEXT    NOT NULL,
  details    TEXT    NOT NULL DEFAULT '{}',
  at         INTEGER NOT NULL
);
CREATE INDEX audit_events_at ON audit_events(at);

CREATE TABLE settings (
  key       TEXT    PRIMARY KEY,
  value     BLOB    NOT NULL,
  encrypted INTEGER NOT NULL DEFAULT 0
);
