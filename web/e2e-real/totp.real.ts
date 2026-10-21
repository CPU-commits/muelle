import { expect, test } from '@playwright/test'
import { TotpClock, totpCode } from './totp'

// El generador de códigos del recorrido (journey.real.ts) contra los vectores de RFC 6238
// (SHA-1, 6 dígitos de los 8 del apéndice B), y la regla del backend: un paso ya usado no
// vuelve a servir (TOTPReplayGuard), y el desfase aceptado es de un paso.
const RFC_SECRET = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ' // "12345678901234567890" en base32

test('los códigos de RFC 6238 (SHA-1)', () => {
	expect(totpCode(RFC_SECRET, 59)).toBe('287082')
	expect(totpCode(RFC_SECRET, 1111111109)).toBe('081804')
	expect(totpCode(RFC_SECRET, 1111111111)).toBe('050471')
	expect(totpCode(RFC_SECRET, 1234567890)).toBe('005924')
	expect(totpCode(RFC_SECRET, 2000000000)).toBe('279037')
})

test('la clave del panel se lee con espacios y en minúsculas', () => {
	expect(totpCode('gezd gnbv gy3t qojq gezd gnbv gy3t qojq', 59)).toBe('287082')
	expect(() => totpCode('no-es-base32!', 59)).toThrow(/base32/)
})

test('cada código sale de un paso nuevo, y a lo más uno adelante del reloj', () => {
	let now = 30 * 1000 + 5 // en ms: el paso 1
	const clock = new TotpClock(() => now)
	const first = clock.next(RFC_SECRET)
	expect(first.step).toBe(1)
	// El mismo paso no se repite: el siguiente es el de adelante, que el backend acepta.
	expect(clock.next(RFC_SECRET).step).toBe(2)
	// Ya no queda uno sin usar dentro del desfase: hay que esperar al paso siguiente.
	expect(clock.waitMs()).toBe(30 * 1000 - 5)
	now += clock.waitMs()
	expect(clock.next(RFC_SECRET).step).toBe(3)
	expect(first.code).toBe(totpCode(RFC_SECRET, 30))
})
