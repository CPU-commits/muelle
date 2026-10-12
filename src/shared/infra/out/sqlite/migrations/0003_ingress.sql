-- Fase 3 (§5.1, §8 y §9 del spec): dominios, del módulo ingress.
-- Reservada para la Fase 3: mientras no haya una versión publicada se puede
-- editar este archivo; las fases no agregan migraciones nuevas.

CREATE TABLE domains (
  id                INTEGER PRIMARY KEY,
  -- RESTRICT: una app con dominios no se borra. Primero se borran sus
  -- dominios, que limpian sus registros de Cloudflare (§9); catalog lo
  -- revisa antes de hacer docker stack rm.
  app_id            INTEGER NOT NULL REFERENCES apps(id) ON DELETE RESTRICT,
  -- En minúsculas, sin punto final. Único en todo muelle.
  hostname          TEXT    NOT NULL UNIQUE,
  -- El nombre corto del servicio (el de la app en una app de formulario).
  target_service    TEXT    NOT NULL,
  target_port       INTEGER NOT NULL CHECK (target_port BETWEEN 1 AND 65535),
  proxied           INTEGER NOT NULL DEFAULT 0 CHECK (proxied IN (0, 1)),
  tls_mode          TEXT    NOT NULL CHECK (tls_mode IN ('cloudflare', 'http')),
  cf_zone_id        TEXT,
  -- NULL: muelle no maneja ese registro (no hay token, la zona no es visible
  -- o el registro ya existía y no se adoptó).
  cf_record_id      TEXT,
  cf_record_id_aaaa TEXT,
  status            TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'error')),
  error_code        TEXT    CHECK (error_code IN ('dns_record_exists', 'dns_error', 'cert_invalid', 'cert_expiring')),
  last_error        TEXT,
  cert_issuer       TEXT,
  cert_expires_at   INTEGER,
  created_at        INTEGER NOT NULL,
  updated_at        INTEGER NOT NULL,
  -- Un dominio en error siempre dice por qué (el panel ofrece Adoptar solo con dns_record_exists).
  CHECK ((status = 'error') = (error_code IS NOT NULL))
);
CREATE INDEX domains_app ON domains(app_id);
