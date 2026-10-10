import { ValidationError } from '@/shared/domain'

// Una imagen tal como la guarda un deploy (spec §5.2 regla 5): `repo:tag@sha256:…`. Antes
// de resolverse (deploy en cola) puede venir sin digest.
export interface ResolvedImage {
	/** `ghcr.io/aumi-app/backend-app:sha-3f9c2e1` */
	readonly ref: string
	/** `sha256:9b2de1f4…`, o null si todavía no se resolvió. */
	readonly digest: string | null
}

export const IMAGE_REF_MAX_LENGTH = 512

// Lo que acepta el override de "Desplegar": empieza con letra o número y solo lleva los
// caracteres de una referencia (`registry:5000/org/app:tag@sha256:…`), sin espacios ni
// nada de shell. La validación de verdad (que exista en el registry) es el paso 2 del
// deploy (§7).
const IMAGE_REF = /^[a-z0-9][a-z0-9._\-/:]*[a-z0-9_](@sha256:[a-f0-9]{64})?$/i

export function splitImageRef(raw: string): ResolvedImage {
	const at = raw.indexOf('@')
	if (at < 0) return { ref: raw, digest: null }
	return { ref: raw.slice(0, at), digest: raw.slice(at + 1) || null }
}

/** Valida un override de imagen escrito en el panel. `field` es el campo del formulario. */
export function parseImageOverride(raw: string, field = 'image'): string {
	const value = raw.trim()
	if (value.length === 0) throw new ValidationError('la imagen está vacía', field)
	if (value.length > IMAGE_REF_MAX_LENGTH || !IMAGE_REF.test(value)) {
		throw new ValidationError(`"${value}" no es una referencia de imagen válida`, field)
	}
	return value
}

export function isValidImageOverride(raw: string): boolean {
	try {
		parseImageOverride(raw)
		return true
	} catch {
		return false
	}
}

/** La parte después del último `/`: `backend-app:sha-3f9c2e1`. */
export function imageName(ref: string): string {
	return ref.slice(ref.lastIndexOf('/') + 1)
}

/** Solo el tag (`sha-3f9c2e1`), o el nombre entero si no tiene. */
export function imageTag(ref: string): string {
	const name = imageName(ref)
	const colon = name.lastIndexOf(':')
	return colon < 0 ? name : name.slice(colon + 1)
}

/** `sha256:9b2de1f4…5d17c07a`: los primeros y los últimos 8 del hash. */
export function shortDigest(digest: string): string {
	const [algorithm, hash] = digest.includes(':') ? digest.split(':', 2) : ['', digest]
	if (!hash || hash.length <= 20) return digest
	const head = `${hash.slice(0, 8)}…${hash.slice(-8)}`
	return algorithm ? `${algorithm}:${head}` : head
}
