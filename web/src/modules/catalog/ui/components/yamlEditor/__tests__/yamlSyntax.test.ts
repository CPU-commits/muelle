import { describe, expect, it } from 'vitest'
import { MOCK_AGENTS_COMPOSE } from '@/mocks/db/catalog'
import { lineCount, yamlSyntaxErrorLines } from '../yamlSyntax'

describe('yamlSyntax', () => {
	it('un compose bien formado no tiene errores (el del diseño, anclas, bloques literales)', () => {
		expect(yamlSyntaxErrorLines(MOCK_AGENTS_COMPOSE)).toEqual([])
		const literal = [
			'x-common: &common',
			'  restart: always',
			'services:',
			'  web:',
			'    <<: *common',
			'    command: >',
			'      sh -c "echo hola',
			'      && run"',
			'    configs: |',
			'      linea 1',
			'      linea 2',
		].join('\n')
		expect(yamlSyntaxErrorLines(literal)).toEqual([])
	})

	it('marca la línea de una indentación rota o un corchete sin cerrar', () => {
		expect(
			yamlSyntaxErrorLines('services:\n  web:\n    image: nginx\n   bad: indent\n'),
		).toEqual([4])
		expect(yamlSyntaxErrorLines('services:\n  web:\n    image: [nginx\n')).toEqual([4])
	})

	it('cuenta las líneas como el editor', () => {
		expect(lineCount('')).toBe(1)
		expect(lineCount('a\nb')).toBe(2)
		expect(lineCount('a\nb\n')).toBe(3)
	})
})
