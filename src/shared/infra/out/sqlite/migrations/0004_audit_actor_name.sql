-- Fase 1, pendiente de su revisión final (§5.1 del spec, contrato §4.2): el
-- nombre del autor de cada evento de auditoría, copiado al escribirlo, como
-- deploys.actor_name: sigue aunque se borre el usuario o el token. El
-- username de una sesión, el nombre de un token, o 'system'.

ALTER TABLE audit_events ADD COLUMN actor_name TEXT NOT NULL DEFAULT '';

-- Los eventos que ya había, cada uno con lo que se sepa de su autor. Los ids
-- de users y api_tokens no se reutilizan (AUTOINCREMENT), así que el de un
-- evento viejo sigue siendo del mismo autor.
UPDATE audit_events SET actor_name = 'system' WHERE actor_type = 'system';

UPDATE audit_events SET actor_name = u.username
  FROM users AS u
 WHERE audit_events.actor_type = 'user' AND u.id = audit_events.actor_id;

-- Un usuario ya borrado dejó su username en el target de su auth.login y de
-- su user.create, los únicos eventos que se escriben a nombre del propio
-- usuario con él en el target (identity, LoginAudit.login y la aceptación de
-- una invitación). Se toma el más nuevo.
UPDATE audit_events SET actor_name = h.name
  FROM (SELECT actor_id, substr(target, 6) AS name, MAX(id)
          FROM audit_events
         WHERE actor_type = 'user' AND action IN ('auth.login', 'user.create') AND target LIKE 'user:_%'
         GROUP BY actor_id) AS h
 WHERE audit_events.actor_type = 'user' AND audit_events.actor_name = ''
   AND h.actor_id = audit_events.actor_id;

UPDATE audit_events SET actor_name = t.name
  FROM api_tokens AS t
 WHERE audit_events.actor_type = 'token' AND t.id = audit_events.actor_id;

-- El resto (un token ya borrado, o un usuario borrado sin esos eventos en los
-- 180 días de retención) queda con '': el panel muestra el tipo y el id.
