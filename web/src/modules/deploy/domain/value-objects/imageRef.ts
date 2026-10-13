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

// --- La gramática de una referencia de Docker -----------------------------------------
//
// Es la del backend, línea por línea: `validate.ImageRef` (fase-23-base) llama a
// `reference.ParseNormalizedNamed` de github.com/distribution/reference v0.6.0 (que
// valida el digest con go-digest v1.0.0). Si el panel acepta algo que el backend rechaza,
// el usuario ve un 422 que se podía evitar; al revés, un deploy frenado sin razón. Los
// tests comparan contra la salida del código de Go (__tests__/imageRefOracle.json).

const alphanumeric = '[a-z0-9]+'
const separator = '(?:[._]|__|[-]+)'
const domainNameComponent = '(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])'
const optionalPort = '(?::[0-9]+)?'
const tagPattern = '[\\w][\\w.-]{0,127}'
const digestPattern = '[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][0-9A-Fa-f]{32,}'
const ipv6address = '\\[(?:[a-fA-F0-9:]+)\\]'
const domainName = `${domainNameComponent}(?:\\.${domainNameComponent})*`
const host = `(?:${domainName}|${ipv6address})`
const domainAndPort = `${host}${optionalPort}`
const pathComponent = `${alphanumeric}(?:${separator}${alphanumeric})*`
const remoteName = `${pathComponent}(?:/${pathComponent})*`
const namePattern = `(?:${domainAndPort}/)?${remoteName}`

const REFERENCE = new RegExp(`^(${namePattern})(?::(${tagPattern}))?(?:@(${digestPattern}))?$`)
const ANCHORED_NAME = new RegExp(`^(?:(${domainAndPort})/)?(${remoteName})$`)
const ANCHORED_IDENTIFIER = /^[a-f0-9]{64}$/

/** El largo máximo de la ruta del repositorio (`RepositoryNameTotalLengthMax`). */
const REPOSITORY_PATH_MAX = 255

/** Los algoritmos de go-digest, con el largo en hexa (minúscula) de cada uno. */
const DIGEST_HEX_LENGTH: Readonly<Record<string, number>> = {
	sha256: 64,
	sha384: 96,
	sha512: 128,
}

const DEFAULT_DOMAIN = 'docker.io'
const LEGACY_DEFAULT_DOMAIN = 'index.docker.io'
const OFFICIAL_REPO_PREFIX = 'library/'

// `splitDockerDomain`: si lo que va antes de la primera `/` es un dominio (localhost,
// tiene `.` o `:`, o lleva mayúsculas) o es parte del nombre en Docker Hub.
function splitDockerDomain(name: string): [string, string] {
	const slash = name.indexOf('/')
	if (slash < 0) return [DEFAULT_DOMAIN, OFFICIAL_REPO_PREFIX + name]
	const maybeDomain = name.slice(0, slash)
	const maybeRemote = name.slice(slash + 1)
	let domain: string
	let remote: string
	if (maybeDomain === 'localhost') [domain, remote] = [maybeDomain, maybeRemote]
	else if (maybeDomain === LEGACY_DEFAULT_DOMAIN) [domain, remote] = [DEFAULT_DOMAIN, maybeRemote]
	else if (/[.:]/.test(maybeDomain)) [domain, remote] = [maybeDomain, maybeRemote]
	else if (maybeDomain.toLowerCase() !== maybeDomain)
		[domain, remote] = [maybeDomain, maybeRemote]
	else [domain, remote] = [DEFAULT_DOMAIN, name]
	if (domain === DEFAULT_DOMAIN && !remote.includes('/')) remote = OFFICIAL_REPO_PREFIX + remote
	return [domain, remote]
}

// go-digest: el algoritmo tiene que existir y el hexa, ser minúscula y de su largo exacto.
function isValidDigest(digest: string): boolean {
	const colon = digest.indexOf(':')
	if (colon <= 0) return false
	const length = DIGEST_HEX_LENGTH[digest.slice(0, colon)]
	const encoded = digest.slice(colon + 1)
	return length !== undefined && encoded.length === length && /^[a-f0-9]+$/.test(encoded)
}

// `reference.Parse`.
function parseReference(value: string): boolean {
	const match = REFERENCE.exec(value)
	if (!match) return false
	const name = match[1] ?? ''
	const path = ANCHORED_NAME.exec(name)?.[2] ?? name
	if (path.length > REPOSITORY_PATH_MAX) return false
	const digest = match[3]
	return digest === undefined || isValidDigest(digest)
}

/**
 * Si `raw` es una referencia de imagen que acepta el backend: registro/repositorio
 * (el repositorio en minúsculas), tag `[\w][\w.-]{0,127}` y digest opcionales.
 */
export function isValidImageRef(raw: string): boolean {
	if (raw === '' || new TextEncoder().encode(raw).length > IMAGE_REF_MAX_LENGTH) return false
	// La defensa extra del backend: nunca algo que parezca una opción o una variable.
	if (raw.startsWith('-') || raw.includes('$')) return false
	if (ANCHORED_IDENTIFIER.test(raw)) return false
	const [domain, remainder] = splitDockerDomain(raw)
	const tagSeparator = remainder.indexOf(':')
	const remote = tagSeparator > -1 ? remainder.slice(0, tagSeparator) : remainder
	if (remote.toLowerCase() !== remote) return false
	return parseReference(`${domain}/${remainder}`)
}

export function splitImageRef(raw: string): ResolvedImage {
	const at = raw.indexOf('@')
	if (at < 0) return { ref: raw, digest: null }
	return { ref: raw.slice(0, at), digest: raw.slice(at + 1) || null }
}

/** Valida un override de imagen escrito en el panel. `field` es el campo del formulario. */
export function parseImageOverride(raw: string, field = 'image'): string {
	const value = raw.trim()
	if (value.length === 0) throw new ValidationError('la imagen está vacía', field)
	if (!isValidImageRef(value)) {
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
