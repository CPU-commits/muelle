-- Lo pidió el panel (contrato §2.1 y §2.11): 1 cuando el paso 7 del deploy
-- no pudo actualizar las rutas de Caddy y escribió la advertencia en su log.
-- Así el panel no decide por el texto del log, que no es un contrato. Es una
-- migración nueva, y no un cambio a 0002_deploy.sql, para que corra también
-- sobre una base que ya tiene deploys.

ALTER TABLE deploys ADD COLUMN routes_warning INTEGER NOT NULL DEFAULT 0 CHECK (routes_warning IN (0, 1));

-- Los deploys que ya había: los succeeded cuyo log trae la advertencia, con
-- la hora adelante (FormatLine: "<RFC 3339 con ms>Z <texto>\n"). En un
-- failed, el texto puede venir de las líneas de una tarea de la app que
-- falló (§2.11), así que no cuenta.
UPDATE deploys SET routes_warning = 1
 WHERE status = 'succeeded'
   AND instr(log, 'Z advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy)' || char(10)) > 0;
