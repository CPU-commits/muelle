import { describe, expect, it } from 'vitest'
import {
	appSlugProblem,
	InvalidSlug,
	parseAppSlug,
	parseProjectSlug,
	parseStageSlug,
	projectSlugProblem,
	slugFromName,
	stageSlugProblem,
} from '../..'

describe('slugs', () => {
	it('acepta los slugs del backend y normaliza mayúsculas y espacios', () => {
		expect(parseProjectSlug(' Auditfy ')).toBe('auditfy')
		expect(parseProjectSlug('aumi-agents')).toBe('aumi-agents')
		expect(parseStageSlug('production')).toBe('production')
		expect(parseAppSlug('a')).toBe('a')
	})

	it('rechaza lo que validate.ProjectSlug rechaza (mismos casos que el test de Go)', () => {
		for (const slug of ['muelle', 'docker', '', 'a_b', '1api', 'api-', 'a'.repeat(17)]) {
			expect(projectSlugProblem(slug), slug).not.toBeNull()
		}
		expect(projectSlugProblem('muelle')).toBe('reserved')
		expect(projectSlugProblem('a_b')).toBe('format')
		expect(projectSlugProblem('a'.repeat(16))).toBeNull()
	})

	it('un stage tiene hasta 12 caracteres y una app hasta 16', () => {
		expect(stageSlugProblem('a'.repeat(12))).toBeNull()
		expect(stageSlugProblem('a-very-long-stage')).toBe('format')
		expect(stageSlugProblem('staging_1')).toBe('format')
		expect(appSlugProblem('a'.repeat(16))).toBeNull()
		expect(appSlugProblem('a'.repeat(17))).toBe('format')
	})

	it('el error dice el problema y el máximo', () => {
		try {
			parseStageSlug('Staging_1')
			expect.unreachable()
		} catch (error) {
			expect(error).toBeInstanceOf(InvalidSlug)
			expect(error).toMatchObject({ problem: 'format', max: 12, field: 'slug' })
		}
		expect(() => parseProjectSlug('docker')).toThrow(InvalidSlug)
	})

	it('sugiere un slug a partir del nombre', () => {
		expect(slugFromName('Aumi Agents', 16)).toBe('aumi-agents')
		expect(slugFromName('  Ñandú & Cía. ', 16)).toBe('nandu-cia')
		expect(slugFromName('Un nombre muy largo de verdad', 16)).toBe('un-nombre-muy-la')
		expect(slugFromName('abcdefghijklmno pq', 16)).toBe('abcdefghijklmno')
	})
})
