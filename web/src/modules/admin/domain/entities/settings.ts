// Los ajustes de `GET /settings` (contrato §4): el 2FA, el proxy, el DNS, los certificados
// y las IPs permitidas para el panel.

/** El token de Cloudflare es de solo escritura: solo se sabe si está y qué ve. */
export interface CloudflareStatus {
	readonly tokenSet: boolean
	/** Cuántas zonas ve el token, si el backend lo sabe. */
	readonly zones: number | null
	/**
	 * Los certificados de Cloudflare (Origin CA, contrato §3.7): con el token y el permiso
	 * Zone → SSL and Certificates → Edit, los dominios con el proxy (nube naranja), también
	 * el del panel, usan uno de Cloudflare en vez de Let's Encrypt. Apagado por defecto.
	 */
	readonly originCa: boolean
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
	/** La IP con la que el backend ve esta sesión (`''` si no la pudo leer). */
	readonly clientIp: string
}

export interface Settings {
	readonly require2fa: boolean
	readonly proxy: ProxySettings
	readonly panelAccess: PanelAccess
}

/**
 * Un `PATCH /settings`: solo van los campos presentes (lo que falta no se toca).
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
	/**
	 * Encender o apagar los certificados de Cloudflare (Origin CA). Quitar el token con la
	 * opción encendida la apaga en el mismo cambio (contrato §4.1).
	 */
	readonly cloudflareOriginCa?: boolean
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

// Un host que se puede mostrar, poner en un link o en un comando sin sorpresas: etiquetas
// de `[a-z0-9-]` separadas por puntos (un dominio o una IPv4), o una IPv6 entre corchetes.
// El URL de WHATWG acepta más (`a$(id).com`, `a;id.com`): esos no pasan.
const HOST_LABEL = '[a-z0-9]([a-z0-9-]*[a-z0-9])?'
const PANEL_HOST = new RegExp(`^(${HOST_LABEL}(\\.${HOST_LABEL})*|\\[[0-9a-f:.]+\\])$`)

/**
 * `text` si es un origen `https://` del panel (un dominio o una IP, con puerto opcional, y
 * nada más: ni ruta, ni usuario, ni mayúsculas), o null. Para lo que se lee de afuera, como
 * el aviso guardado en el localStorage.
 */
export function parsePanelOrigin(text: string): string | null {
	let url: URL
	try {
		url = new URL(text)
	} catch {
		return null
	}
	if (url.protocol !== 'https:' || url.origin !== text) return null
	return PANEL_HOST.test(url.hostname) ? text : null
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
 * Volver a entrar después de una lista de IPs que deja fuera (contrato §5.4): corre adentro
 * del contenedor de muelle, vacía `panel_allowed_cidrs` y reinicia el servicio, que al
 * arrancar le reenvía la configuración a Caddy.
 */
export const RESET_PANEL_IPS_COMMAND =
	'docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips'
