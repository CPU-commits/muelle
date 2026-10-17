/**
 * El origen de esta página (`''` fuera del navegador). Va aparte para que un test pueda
 * entrar "por la dirección de Caddy" (`https://<panel_domain>`), que no es la de vitest.
 */
export function currentOrigin(): string {
	return typeof location === 'undefined' ? '' : location.origin
}
