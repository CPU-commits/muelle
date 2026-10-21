import { InvalidCloudflareToken, RequiredField } from '../errors/adminErrors'

// La forma que el backend exige antes de preguntarle a Cloudflare (contrato §4.1): de 20 a
// 512 caracteres visibles, sin espacios. Que Cloudflare lo acepte y vea zonas lo dice el
// backend (422 `cloudflare_token`).
export const CLOUDFLARE_TOKEN_MIN_LENGTH = 20
export const CLOUDFLARE_TOKEN_MAX_LENGTH = 512

const VISIBLE = /^[\x21-\x7e]+$/

/** Sin los espacios de los bordes (lo que trae un pegado). Vacío es obligatorio. */
export function parseCloudflareToken(raw: string): string {
	const token = raw.trim()
	if (token === '') throw new RequiredField('cloudflareToken')
	if (/\s/.test(token) || !VISIBLE.test(token)) throw new InvalidCloudflareToken('spaces')
	if (token.length < CLOUDFLARE_TOKEN_MIN_LENGTH || token.length > CLOUDFLARE_TOKEN_MAX_LENGTH) {
		throw new InvalidCloudflareToken('length')
	}
	// Caddy lo leería como un placeholder (contrato §3.4): el backend lo rechaza con un 422
	// (§4.1), y el panel lo frena antes.
	if (/[{}]/.test(token)) throw new InvalidCloudflareToken('placeholder')
	return token
}
