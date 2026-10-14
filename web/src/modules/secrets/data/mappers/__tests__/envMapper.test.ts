import { describe, expect, it } from 'vitest'
import { parseEnvPatch, parseEnvReplacement } from '../../../domain'
import { envDtoSchema } from '../../dtos/envDto'
import { envFromDto, envVarFromDto, patchPayload, replacePayload } from '../envMapper'

describe('envMapper', () => {
	it('traduce una variable de snake_case al dominio, con la fecha como Date', () => {
		expect(
			envVarFromDto({
				key: 'PORT',
				value: '3000',
				secret: false,
				updated_at: '2026-10-10T09:00:00.123456789-03:00',
			}),
		).toEqual({
			key: 'PORT',
			secret: false,
			value: '3000',
			updatedAt: new Date('2026-10-10T12:00:00.123Z'),
		})
	})

	it('una secreta queda sin valor', () => {
		expect(
			envFromDto({
				vars: [
					{ key: 'JWT', value: null, secret: true, updated_at: '2026-10-10T12:00:00Z' },
				],
			}),
		).toEqual([
			{ key: 'JWT', secret: true, value: null, updatedAt: new Date('2026-10-10T12:00:00Z') },
		])
	})

	it('el schema acepta la respuesta de Go y rechaza un secreto con valor', () => {
		const ok = envDtoSchema.safeParse({
			vars: [
				{ key: 'A', value: '', secret: false, updated_at: '2026-10-10T12:00:00Z' },
				{ key: 'B', value: null, secret: true, updated_at: '2026-10-10T09:00:00-03:00' },
			],
		})
		expect(ok.success).toBe(true)

		for (const bad of [
			{ key: 'B', value: 'filtrado', secret: true, updated_at: '2026-10-10T12:00:00Z' },
			{ key: 'C', value: null, secret: false, updated_at: '2026-10-10T12:00:00Z' },
			{ key: 'D', value: 'x', secret: false, updated_at: 'ayer' },
		]) {
			expect(envDtoSchema.safeParse({ vars: [bad] }).success, bad.key).toBe(false)
		}
	})

	it('el PATCH lleva solo los campos que usa, con los nombres exactos del backend', () => {
		expect(
			patchPayload(parseEnvPatch({ set: [['A', '1']], unset: ['B'], secret: false })),
		).toEqual({ set: { A: '1' }, unset: ['B'], secret: false })
		expect(patchPayload(parseEnvPatch({ unset: ['B'], secret: true }))).toEqual({
			unset: ['B'],
		})
		expect(patchPayload(parseEnvPatch({ set: [['A', '']], secret: true }))).toEqual({
			set: { A: '' },
			secret: true,
		})
	})

	it('__proto__ viaja como una clave más', () => {
		const body = patchPayload(parseEnvPatch({ set: [['__proto__', 'x']], secret: true }))
		expect(JSON.stringify(body)).toBe('{"set":{"__proto__":"x"},"secret":true}')
		expect(
			JSON.stringify(
				replacePayload(parseEnvReplacement({ vars: [['__proto__', 'y']], secret: false })),
			),
		).toBe('{"vars":{"__proto__":"y"},"secret":false}')
	})

	it('el PUT siempre manda vars y secret', () => {
		expect(replacePayload(parseEnvReplacement({ vars: [], secret: true }))).toEqual({
			vars: {},
			secret: true,
		})
	})
})
