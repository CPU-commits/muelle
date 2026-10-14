import { describe, expect, it } from 'vitest'
import { InvalidProjectName, parseProjectName, projectNameProblem } from '../..'

describe('projectName', () => {
	it('acepta un nombre y le saca los espacios de los bordes', () => {
		expect(parseProjectName('  Auditfy  ')).toBe('Auditfy')
		expect(parseProjectName('Aumi · Agents ❤')).toBe('Aumi · Agents ❤')
	})

	it('cuenta caracteres, no bytes: 64 emoji o acentos pasan', () => {
		expect(projectNameProblem('é'.repeat(64))).toBeNull()
		expect(projectNameProblem('é'.repeat(65))).toBe('long')
	})

	it('rechaza vacíos, invisibles, controles y el modificador de los emoji', () => {
		expect(projectNameProblem('   ')).toBe('empty')
		for (const name of [
			'a\u200bb',
			'a\u202eb',
			'a\u0007b',
			'\u2800',
			'x\u3164',
			'\u2764\ufe0f',
		]) {
			expect(projectNameProblem(name), JSON.stringify(name)).toBe('invisible')
		}
		expect(() => parseProjectName('')).toThrow(InvalidProjectName)
	})
})
