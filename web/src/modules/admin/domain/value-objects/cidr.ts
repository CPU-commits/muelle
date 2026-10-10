import { InvalidCidr } from '../errors/adminErrors'
import { type IpValue, parseIp } from './ipAddress'

// `panel_allowed_cidrs`: las IPs o rangos desde los que se puede abrir el panel (el
// matcher `remote_ip` de Caddy, spec §8). En la pantalla es un textarea, uno por línea.

interface CidrValue {
	readonly network: IpValue
	readonly prefix: number
}

const BITS = { 4: 32, 6: 128 } as const

function parseCidr(raw: string): CidrValue | null {
	const [address = '', prefixText, ...extra] = raw.trim().split('/')
	if (extra.length > 0) return null
	const network = parseIp(address)
	if (!network) return null
	const bits = BITS[network.version]
	if (prefixText === undefined) return { network, prefix: bits }
	if (!/^\d{1,3}$/.test(prefixText)) return null
	const prefix = Number(prefixText)
	return prefix <= bits ? { network, prefix } : null
}

function normalize(raw: string): string {
	return raw.trim().toLowerCase()
}

/** Las líneas no vacías del textarea, sin espacios en los bordes. */
function entries(text: string): { line: number; value: string }[] {
	return text
		.split('\n')
		.map((value, index) => ({ line: index + 1, value: normalize(value) }))
		.filter((entry) => entry.value !== '')
}

/** Valida cada línea; la primera que no sirve sale como `InvalidCidr` con su número. */
export function parseCidrList(text: string): string[] {
	return entries(text).map(({ line, value }) => {
		if (!parseCidr(value)) throw new InvalidCidr(line, value)
		return value
	})
}

/** El valor del textarea para una lista guardada. */
export function cidrListText(list: readonly string[]): string {
	return list.join('\n')
}

/** Si el textarea dice lo mismo que la lista guardada (sin contar líneas vacías). */
export function sameCidrList(text: string, list: readonly string[]): boolean {
	const current = entries(text).map((entry) => entry.value)
	return current.length === list.length && current.every((value, i) => value === list[i])
}

export function cidrContains(cidr: string, ip: string): boolean {
	const range = parseCidr(cidr)
	const address = parseIp(ip)
	if (!range || !address || range.network.version !== address.version) return false
	const shift = BigInt(BITS[address.version] - range.prefix)
	return range.network.value >> shift === address.value >> shift
}

/**
 * Si con esta lista `ip` todavía puede abrir el panel. Una lista vacía deja entrar a
 * cualquiera; una lista inválida no se puede guardar, así que se responde por lo válido.
 */
export function cidrListAllows(text: string, ip: string): boolean {
	const list = entries(text)
	if (list.length === 0) return true
	return list.some((entry) => cidrContains(entry.value, ip))
}
