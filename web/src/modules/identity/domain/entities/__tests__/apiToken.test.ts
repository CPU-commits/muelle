import { describe, expect, it } from 'vitest'
import {
	actionAppPath,
	isTokenExpired,
	MissingTokenProject,
	parseTokenScope,
	partitionTokens,
	scopeLabel,
	type TokenId,
} from '../..'
import { fakeToken } from '../../repositories/__fakes__/inMemoryIdentityRepository'

const NOW = new Date('2026-10-10T12:00:00.000Z')

describe('apiToken', () => {
	it('el alcance es el proyecto, o el proyecto y un stage', () => {
		expect(parseTokenScope('auditfy', '')).toEqual({ project: 'auditfy', stage: null })
		expect(parseTokenScope(' auditfy ', 'staging')).toEqual({
			project: 'auditfy',
			stage: 'staging',
		})
		expect(() => parseTokenScope('', 'staging')).toThrow(MissingTokenProject)
	})

	it('muestra el alcance como en el diseño', () => {
		expect(scopeLabel({ project: 'auditfy', stage: null })).toBe('auditfy')
		expect(scopeLabel({ project: 'extractor', stage: 'staging' })).toBe('extractor/staging')
	})

	it('el ejemplo de la action deja como marcador lo que el token no sabe', () => {
		expect(actionAppPath({ project: 'aumi', stage: 'staging' })).toBe('aumi/staging/<app>')
		expect(actionAppPath({ project: 'aumi', stage: null })).toBe('aumi/<stage>/<app>')
	})

	it('vence en el instante de expiresAt', () => {
		expect(isTokenExpired(fakeToken(), NOW)).toBe(false)
		expect(isTokenExpired(fakeToken({ expiresAt: NOW }), NOW)).toBe(true)
		expect(isTokenExpired(fakeToken({ expiresAt: new Date('2026-12-31') }), NOW)).toBe(false)
	})

	it('separa vigentes de vencidos sin cambiar el orden', () => {
		const a = fakeToken({ id: 1 as TokenId })
		const b = fakeToken({ id: 2 as TokenId, expiresAt: new Date('2026-01-01') })
		const c = fakeToken({ id: 3 as TokenId, expiresAt: new Date('2027-01-01') })

		expect(partitionTokens([a, b, c], NOW)).toEqual({ active: [a, c], expired: [b] })
	})
})
