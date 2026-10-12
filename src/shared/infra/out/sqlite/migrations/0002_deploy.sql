-- Fase 2 (§5.1 y §7 del spec): deploys y registries, del módulo deploy.
-- Reservada para la Fase 2: mientras no haya una versión publicada se puede
-- editar este archivo; las fases no agregan migraciones nuevas.

-- AUTOINCREMENT: el id de un deploy sale en la API (status_url, la action de
-- GitHub) y en la auditoría, así que uno borrado por la retención (los
-- últimos 20 por app) no se reutiliza.
CREATE TABLE deploys (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  app_id        INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  status        TEXT    NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'superseded')),
  -- JSON servicio → imagen. Mientras el deploy está queued son las refs que
  -- se van a desplegar; desde que se resuelven (§7 paso 2), ref@sha256:….
  images        TEXT    NOT NULL DEFAULT '{}',
  trigger       TEXT    NOT NULL CHECK (trigger IN ('panel', 'api', 'rollback')),
  actor_type    TEXT    NOT NULL CHECK (actor_type IN ('user', 'token')),
  actor_id      INTEGER,
  -- El username o el nombre del token al crear el deploy: se sigue leyendo
  -- aunque el usuario o el token se borren.
  actor_name    TEXT    NOT NULL DEFAULT '',
  -- El deploy más nuevo de la misma app que dejó a este superseded.
  superseded_by INTEGER REFERENCES deploys(id) ON DELETE SET NULL,
  error         TEXT,
  -- Texto con un timestamp por línea, ya enmascarado, truncado a 256 KB.
  log           TEXT    NOT NULL DEFAULT '',
  created_at    INTEGER NOT NULL,
  started_at    INTEGER,
  finished_at   INTEGER
);
CREATE INDEX deploys_app ON deploys(app_id, id);
-- La reconciliación al arrancar busca los queued y running (§7).
CREATE INDEX deploys_active ON deploys(status) WHERE status IN ('queued', 'running');

-- AUTOINCREMENT: el id va en la AAD de la contraseña, registries:password:<id> (§12).
CREATE TABLE registries (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  host       TEXT    NOT NULL UNIQUE,
  username   TEXT    NOT NULL,
  password   BLOB    NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
