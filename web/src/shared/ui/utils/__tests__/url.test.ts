import { describe, expect, it } from 'vitest'
import { withQuery } from '../url'

describe('withQuery', () => {
	it('agrega el query string a una ruta ya resuelta', () => {
		expect(withQuery('/projects/auditfy/stages/staging/apps/api/deploys', { deploy: 42 })).toBe(
			'/projects/auditfy/stages/staging/apps/api/deploys?deploy=42',
		)
	})

	it('codifica los valores y saltea los null y undefined', () => {
		expect(
			withQuery('/settings', { q: 'a b&c', page: null, before: undefined, all: true }),
		).toBe('/settings?q=a+b%26c&all=true')
	})

	it('sin nada que agregar devuelve la ruta tal cual', () => {
		expect(withQuery('/settings', {})).toBe('/settings')
		expect(withQuery('/settings', { q: null })).toBe('/settings')
	})

	it('respeta un query que ya estaba y deja el hash al final', () => {
		expect(withQuery('/x?tab=logs#fin', { deploy: 7 })).toBe('/x?tab=logs&deploy=7#fin')
	})
})
