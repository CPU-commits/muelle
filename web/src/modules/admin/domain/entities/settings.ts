import type { RoadmapPhase } from '@/shared/domain'

// Los ajustes de `GET /settings`. En la Fase 1 el backend solo tiene `require_2fa`; el
// resto (proxy, DNS, certificados e IPs permitidas, spec §5.1 y §8–9) llega con la Fase 3
// como campos nuevos del mismo endpoint. Mientras falten, la sección es `null` y la
// pantalla muestra "Llega con la Fase 3".

/** La fase en la que llegan `proxy` y `panelAccess`. */
export const PROXY_SETTINGS_PHASE: RoadmapPhase = 3

/** El token de Cloudflare es de solo escritura: solo se sabe si está y qué ve. */
export interface CloudflareStatus {
	readonly tokenSet: boolean
	/** Cuántas zonas ve el token, si el backend lo sabe. */
	readonly zones: number | null
}

export interface ProxySettings {
	readonly cloudflare: CloudflareStatus
	/** `''` si todavía no se configuró. */
	readonly publicIpv4: string
	/** Opcional: `''` es sin IPv6. */
	readonly publicIpv6: string
	readonly acmeEmail: string
	/** Se define al instalar; el panel solo lo muestra. */
	readonly panelDomain: string
	readonly httpPort: number
	readonly httpsPort: number
}

export interface PanelAccess {
	/** Vacía: el panel se abre desde cualquier IP. */
	readonly allowedCidrs: readonly string[]
	/** La IP con la que el backend ve esta sesión (`''` si no la informó). */
	readonly clientIp: string
}

export interface Settings {
	readonly require2fa: boolean
	/** `null`: el backend todavía no tiene la Fase 3. */
	readonly proxy: ProxySettings | null
	/** `null`: el backend todavía no tiene la Fase 3. */
	readonly panelAccess: PanelAccess | null
}

/**
 * Un `PATCH /settings`: solo van los campos presentes. Uno que el backend no tiene
 * todavía responde 400, así que la pantalla no los manda mientras su sección es `null`.
 */
export interface SettingsPatch {
	readonly require2fa?: boolean
	readonly publicIpv4?: string
	readonly publicIpv6?: string
	readonly acmeEmail?: string
	readonly httpPort?: number
	readonly httpsPort?: number
	readonly panelAllowedCidrs?: readonly string[]
	/** `''` quita el token (y las zonas que veía). */
	readonly cloudflareToken?: string
	/** Guardar una lista de IPs que deja fuera esta sesión (si no, `WouldLockOut`). */
	readonly confirmLockout?: boolean
}

/**
 * Dónde sirve muelle-caddy el panel: `https://<panel_domain>`, con `:<puerto>` si el HTTPS no
 * es 443 (`ProxySettings.PanelOrigin`, contrato §0.1 y §3.5). null sin dominio del panel (una
 * IP o `localhost`, en desarrollo): ahí Caddy no sirve el panel.
 */
export function panelOrigin(
	proxy: Pick<ProxySettings, 'panelDomain' | 'httpsPort'>,
): string | null {
	if (proxy.panelDomain === '') return null
	const port = proxy.httpsPort === 443 ? '' : `:${String(proxy.httpsPort)}`
	return `https://${proxy.panelDomain}${port}`
}

/**
 * El origen nuevo del panel después de cambiar los puertos, o null si sigue donde estaba:
 * solo lo mueve el puerto HTTPS, y solo si Caddy sirve el panel (contrato §3.5).
 */
export function movedPanelOrigin(before: ProxySettings, after: ProxySettings): string | null {
	if (before.httpsPort === after.httpsPort) return null
	return panelOrigin(after)
}

/**
 * Lo que actualiza `MUELLE_PANEL_ORIGIN` en el servidor (contrato §5.2): install lo fijó con
 * el puerto HTTPS de entonces. Reinicia muelle unos segundos.
 */
export function updatePanelOriginCommand(origin: string): string {
	return `docker service update --env-add MUELLE_PANEL_ORIGIN=${origin} muelle_muelle`
}

/**
 * Volver a entrar después de una lista de IPs que deja fuera (contrato §5.4): corre adentro
 * del contenedor de muelle, vacía `panel_allowed_cidrs` y reinicia el servicio, que al
 * arrancar le reenvía la configuración a Caddy.
 */
export const RESET_PANEL_IPS_COMMAND =
	'docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips'
