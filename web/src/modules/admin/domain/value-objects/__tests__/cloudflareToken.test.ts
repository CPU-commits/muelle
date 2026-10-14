import { describe, expect, it } from 'vitest'
import { InvalidCloudflareToken, parseCloudflareToken, RequiredField } from '../..'

// La forma del token que valida el backend antes de preguntarle a Cloudflare (contrato §4.1):
// de 20 a 512 caracteres visibles, sin espacios.
describe('cloudflareToken', () => {
	const valid = 'x'.repeat(40)

	it('acepta un token con forma de token, sin los espacios de los bordes', () => {
		expect(parseCloudflareToken(` ${valid}\n`)).toBe(valid)
		expect(parseCloudflareToken('a'.repeat(20))).toBe('a'.repeat(20))
		expect(parseCloudflareToken('a'.repeat(512))).toBe('a'.repeat(512))
	})

	it('vacío es obligatorio: para quitar el token hay otro botón', () => {
		expect(() => parseCloudflareToken('  ')).toThrow(RequiredField)
	})

	it('rechaza el largo equivocado y los espacios del medio', () => {
		expect(() => parseCloudflareToken('corto')).toThrow(
			expect.objectContaining({ name: 'InvalidCloudflareToken', problem: 'length' }),
		)
		expect(() => parseCloudflareToken('a'.repeat(513))).toThrow(InvalidCloudflareToken)
		expect(() => parseCloudflareToken(`${valid} ${valid}`)).toThrow(
			expect.objectContaining({ problem: 'spaces', field: 'cloudflareToken' }),
		)
	})
})
