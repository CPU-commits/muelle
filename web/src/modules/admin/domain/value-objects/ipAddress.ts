import { InvalidIpAddress } from '../errors/adminErrors'

// Direcciones IP como números, para validar la IP pública y saber si un rango CIDR
// incluye una dirección. Sin zonas (`fe80::1%eth0`): no tienen sentido en un servidor
// público ni en un matcher `remote_ip` de Caddy.

export interface IpValue {
	readonly version: 4 | 6
	readonly value: bigint
}

const IPV4_OCTET = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$/
const IPV6_GROUP = /^[0-9a-f]{1,4}$/

function ipv4ToBigInt(raw: string): bigint | null {
	const parts = raw.split('.')
	if (parts.length !== 4 || !parts.every((part) => IPV4_OCTET.test(part))) return null
	return parts.reduce((acc, part) => (acc << 8n) | BigInt(Number(part)), 0n)
}

function groupsToBigInt(groups: readonly string[]): bigint | null {
	if (!groups.every((group) => IPV6_GROUP.test(group))) return null
	return groups.reduce((acc, group) => (acc << 16n) | BigInt(Number.parseInt(group, 16)), 0n)
}

/** Los 8 grupos de 16 bits de una IPv6, con `::` expandido y una IPv4 final como 2 grupos. */
function ipv6Groups(raw: string): string[] | null {
	let address = raw.toLowerCase()
	// `::ffff:192.0.2.1`: la IPv4 del final se escribe como sus dos grupos (`c000:201`).
	const lastColon = address.lastIndexOf(':')
	const last = address.slice(lastColon + 1)
	if (last.includes('.')) {
		const v4 = ipv4ToBigInt(last)
		if (v4 === null) return null
		const high = ((v4 >> 16n) & 0xffffn).toString(16)
		const low = (v4 & 0xffffn).toString(16)
		address = `${address.slice(0, lastColon + 1)}${high}:${low}`
	}

	const halves = address.split('::')
	if (halves.length > 2) return null
	const split = (part: string): string[] => (part === '' ? [] : part.split(':'))
	const head = split(halves[0] ?? '')
	if (halves.length === 1) return head.length === 8 ? head : null
	const tail = split(halves[1] ?? '')
	const explicit = head.length + tail.length
	// `::` reemplaza al menos un grupo.
	if (explicit > 7) return null
	return [...head, ...Array<string>(8 - explicit).fill('0'), ...tail]
}

export function parseIp(raw: string): IpValue | null {
	const address = raw.trim()
	if (address.includes(':')) {
		const groups = ipv6Groups(address)
		const value = groups ? groupsToBigInt(groups) : null
		return value === null ? null : { version: 6, value }
	}
	const value = ipv4ToBigInt(address)
	return value === null ? null : { version: 4, value }
}

export function isIpv4(raw: string): boolean {
	return parseIp(raw)?.version === 4
}

export function isIpv6(raw: string): boolean {
	return parseIp(raw)?.version === 6
}

/** La IPv4 pública, obligatoria. */
export function parsePublicIpv4(raw: string): string {
	const address = raw.trim()
	if (!isIpv4(address)) throw new InvalidIpAddress(4, 'ipv4')
	return address
}

/** La IPv6 pública es opcional: `''` la borra. */
export function parsePublicIpv6(raw: string): string {
	const address = raw.trim().toLowerCase()
	if (address === '') return ''
	if (!isIpv6(address)) throw new InvalidIpAddress(6, 'ipv6')
	return address
}
