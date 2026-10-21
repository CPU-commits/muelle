import { z } from 'zod'

// Las formas de admin: settings y auditoría de la Fase 1 (src/internal/admin/app/dto), y
// lo que agregan las Fases 2 y 3 según el contrato (§2.10 los registries, §4 los ajustes del
// proxy). Ver web/CLAUDE.md, "Endpoints de las fases 2 y 3".

// UTC con milisegundos (`2026-10-10T19:36:53.770Z`) o RFC 3339 completo con offset.
const timestamp = z.iso.datetime({ offset: true })

/**
 * `GET /settings` y la respuesta de `PATCH /settings` (contrato §4): con la Fase 3 trae
 * siempre todas las claves. Son opcionales solo por un backend de la Fase 1 (`pnpm dev`
 * contra `main`), que tiene nada más que `require_2fa`. `caddy_image` no es un ajuste: solo
 * lo fija install, y si viniera, Zod lo descarta.
 */
export const settingsDtoSchema = z.object({
	require_2fa: z.boolean(),
	/** El token de Cloudflare es de solo escritura: el GET solo dice si está. */
	cloudflare_token_set: z.boolean().optional(),
	/** Cuántas zonas ve el token; null si no se pudo consultar. */
	cloudflare_zones: z.number().int().nonnegative().nullable().optional(),
	/** `''` si no está configurada. */
	public_ip: z.string().optional(),
	public_ipv6: z.string().optional(),
	acme_email: z.string().optional(),
	panel_domain: z.string().optional(),
	proxy_http_port: z.number().int().optional(),
	proxy_https_port: z.number().int().optional(),
	panel_allowed_cidrs: z.array(z.string()).optional(),
	/** La IP con la que el backend ve la petición: siempre un string (no es un ajuste). */
	client_ip: z.string().optional(),
})

/**
 * Un evento de `GET /audit`. `details` no se declara a propósito: Zod lo descarta y no
 * llega ni al dominio ni a la pantalla.
 */
export const auditEventDtoSchema = z.object({
	id: z.number().int().positive(),
	actor_type: z.string(),
	actor_id: z.number().int().nullable(),
	// El username, el nombre del token o "system", copiado al escribir el evento (contrato
	// §4.2): nunca null. Opcional solo por un backend de solo la Fase 1, que no lo tiene.
	actor_name: z.string().optional(),
	action: z.string(),
	target: z.string(),
	at: timestamp,
})

/** Un elemento de `GET /registries` (Fase 2, contrato §2.10). La contraseña no vuelve nunca. */
export const registryDtoSchema = z.object({
	host: z.string().min(1),
	username: z.string(),
	created_at: timestamp,
	updated_at: timestamp,
})

export type SettingsDto = z.infer<typeof settingsDtoSchema>
export type AuditEventDto = z.infer<typeof auditEventDtoSchema>
export type RegistryDto = z.infer<typeof registryDtoSchema>

/** Cuerpo de `PATCH /settings`: un campo que falta no se toca (`PatchSettingsInput`). */
export interface SettingsPatchPayload {
	require_2fa?: boolean
	public_ip?: string
	public_ipv6?: string
	acme_email?: string
	proxy_http_port?: number
	proxy_https_port?: number
	panel_allowed_cidrs?: string[]
	/** `""` quita el token y las zonas. */
	cloudflare_token?: string
	/** Guardar una lista que deja fuera esta sesión (si no, `409 would_lock_out`). */
	confirm_lockout?: boolean
}

/** Cuerpo de `POST /registries`. */
export interface NewRegistryPayload {
	host: string
	username: string
	password: string
}

/** Cuerpo de `PATCH /registries/{host}`. */
export interface RegistryCredentialsPayload {
	username: string
	password: string
}
