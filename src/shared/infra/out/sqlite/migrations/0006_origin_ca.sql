-- Certificados de Cloudflare (Origin CA), opcionales (contrato §3.7): los que
-- emitió muelle y lo que vio de cada hostname en Cloudflare. Es una migración
-- nueva, y no un cambio a 0003_ingress.sql, para que corra también sobre una
-- base que ya tiene dominios.

-- AUTOINCREMENT: el id va en la AAD de la clave,
-- origin_ca_certs:private_key:<id> (§12 del diseño).
CREATE TABLE origin_ca_certs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  -- El de un dominio o el del panel. Sin FK: el panel no está en domains, y
  -- un certificado retirado sobrevive a su dominio hasta que se revoca.
  hostname        TEXT    NOT NULL,
  zone_id         TEXT    NOT NULL,
  -- El id de Cloudflare (el serial en decimal), para revocarlo.
  cf_cert_id      TEXT    NOT NULL UNIQUE,
  certificate     TEXT    NOT NULL,  -- PEM de la hoja
  -- PKCS#8 en PEM, cifrado con la clave maestra. x'' en uno retirado: ya no
  -- se necesita.
  private_key     BLOB    NOT NULL,
  expires_at      INTEGER NOT NULL,  -- NotAfter, en ms
  state           TEXT    NOT NULL CHECK (state IN ('active', 'retired')),
  created_at      INTEGER NOT NULL,
  retired_at      INTEGER,
  revoke_failures INTEGER NOT NULL DEFAULT 0,
  next_revoke_at  INTEGER,
  CHECK ((state = 'retired') = (retired_at IS NOT NULL))
);
-- A lo más un certificado activo por hostname.
CREATE UNIQUE INDEX origin_ca_certs_active ON origin_ca_certs(hostname) WHERE state = 'active';

CREATE TABLE origin_ca_hosts (
  hostname        TEXT    PRIMARY KEY,
  -- Lo último que se vio en Cloudflare (contrato §3.7); NULL: todavía no se miró.
  observed        TEXT    CHECK (observed IN ('proxied', 'dns_only', 'missing', 'unreadable')),
  observed_at     INTEGER,
  -- La última emisión o renovación que falló, y cuándo se reintenta.
  failures        INTEGER NOT NULL DEFAULT 0,
  last_error_code TEXT    CHECK (last_error_code IN ('permission', 'cloudflare', 'rejected')),
  -- Solo la causa; el mensaje entero se arma al leer, según el status (§9.3).
  last_error      TEXT,
  next_attempt_at INTEGER,
  updated_at      INTEGER NOT NULL,
  CHECK ((last_error_code IS NULL) = (last_error IS NULL)),
  CHECK ((failures = 0) = (last_error_code IS NULL)),
  -- Con una falla, siempre hay un reintento: retry_at nunca es null con error_code.
  CHECK ((failures = 0) = (next_attempt_at IS NULL))
);
