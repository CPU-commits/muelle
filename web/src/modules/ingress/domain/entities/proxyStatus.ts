// El estado del proxy de muelle (muelle-caddy), para el aviso "proxy desincronizado" (spec
// §8 y §13; contrato §3.3, `GET /proxy`). Es de la Fase 3: sin ella, no se muestra nada.

/**
 * - `synced`: la última configuración se aplicó.
 * - `out_of_sync`: la última falló, y se reintenta con backoff.
 * - `recreating`: muelle-caddy se está recreando porque cambiaron los puertos en un
 *   `PATCH /settings` (la imagen nunca cambia por un PATCH, contrato §3.5).
 * - `disabled`: sin dominio del panel o sin imagen de Caddy (en desarrollo).
 */
export const PROXY_SYNC_STATUSES = ['synced', 'out_of_sync', 'recreating', 'disabled'] as const
export type ProxySyncStatus = (typeof PROXY_SYNC_STATUSES)[number]

export interface CaddyState {
	/** La imagen que corre de verdad. */
	readonly image: string | null
	readonly running: boolean
	readonly httpPort: number | null
	readonly httpsPort: number | null
}

export interface ProxyStatus {
	readonly status: ProxySyncStatus
	/** El motivo de `out_of_sync`, o del último fallo del contenedor. */
	readonly error: string | null
	/** La última configuración que se aplicó. */
	readonly syncedAt: Date | null
	readonly caddy: CaddyState
}

/**
 * El aviso que va arriba de Dominios, o null si el proxy anda:
 * - `out_of_sync`: Caddy no aceptó la última configuración (muelle reintenta solo);
 * - `stopped`: sincronizado, pero muelle-caddy no está corriendo;
 * - `recreating`: se está recreando; vuelve en unos segundos;
 * - `disabled`: apagado, los dominios no reciben tráfico.
 */
export type ProxyNotice = 'out_of_sync' | 'stopped' | 'recreating' | 'disabled'

export function proxyNotice(proxy: ProxyStatus | null | undefined): ProxyNotice | null {
	if (!proxy) return null
	if (proxy.status === 'out_of_sync') return 'out_of_sync'
	if (proxy.status === 'recreating') return 'recreating'
	if (proxy.status === 'disabled') return 'disabled'
	return proxy.caddy.running ? null : 'stopped'
}

/** Mientras el proxy se está arreglando solo, se pregunta de nuevo cada tanto. */
export function proxySettling(proxy: ProxyStatus | null | undefined): boolean {
	return proxy?.status === 'out_of_sync' || proxy?.status === 'recreating'
}
