import { createHmac } from 'node:crypto'

// Códigos TOTP como los valida muelle (src/internal/identity/infra/out/service/totp.go):
// RFC 6238 con SHA-1, 6 dígitos y pasos de 30 s, con un paso de desfase. El panel muestra
// la clave en base32, en grupos de 4 (`#totp-secret` de /2fa).

const PERIOD_S = 30
const BASE32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

function base32Decode(text: string): Buffer {
	const clean = text.replace(/[\s=]/g, '').toUpperCase()
	let bits = 0
	let value = 0
	const out: number[] = []
	for (const char of clean) {
		const index = BASE32.indexOf(char)
		if (index < 0) throw new Error(`la clave TOTP no es base32: "${char}"`)
		value = (value << 5) | index
		bits += 5
		if (bits >= 8) {
			out.push((value >>> (bits - 8)) & 0xff)
			bits -= 8
		}
	}
	return Buffer.from(out)
}

/** El código de 6 dígitos de `secret` (base32) en el instante `unixSeconds`. */
export function totpCode(secret: string, unixSeconds: number): string {
	return codeForStep(base32Decode(secret), Math.floor(unixSeconds / PERIOD_S))
}

function codeForStep(key: Buffer, step: number): string {
	const counter = Buffer.alloc(8)
	counter.writeBigUInt64BE(BigInt(step))
	const mac = createHmac('sha1', key).update(counter).digest()
	const offset = (mac[mac.length - 1] ?? 0) & 0x0f
	const binary = mac.readUInt32BE(offset) & 0x7fffffff
	return String(binary % 1_000_000).padStart(6, '0')
}

/**
 * Da códigos que muelle acepta una sola vez: cada uno de un paso posterior al anterior
 * (TOTPReplayGuard) y a lo más un paso adelante del reloj (el desfase que tolera). Si no
 * queda ninguno, `waitMs()` dice cuánto falta para el paso siguiente.
 */
export class TotpClock {
	#last = -1
	readonly #now: () => number

	constructor(now: () => number = Date.now) {
		this.#now = now
	}

	#current(): number {
		return Math.floor(this.#now() / 1000 / PERIOD_S)
	}

	/** Cuánto esperar (ms) antes de que `next` tenga un paso sin usar; 0 si ya lo tiene. */
	waitMs(): number {
		const current = this.#current()
		if (this.#last < current + 1) return 0
		return (current + 1) * PERIOD_S * 1000 - this.#now()
	}

	next(secret: string): { code: string; step: number } {
		if (this.waitMs() > 0) throw new Error('no queda un paso TOTP sin usar: esperar waitMs()')
		const step = Math.max(this.#current(), this.#last + 1)
		this.#last = step
		return { code: codeForStep(base32Decode(secret), step), step }
	}
}
