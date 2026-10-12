import { describe, expect, it } from 'vitest'
import { formatCommandLine, parseCommandLine } from '../..'

describe('commandLine', () => {
	it('separa por espacios y respeta las comillas', () => {
		expect(parseCommandLine('node dist/x.js')).toEqual(['node', 'dist/x.js'])
		expect(parseCommandLine("  sh -c 'curl -f http://x || exit 1' ")).toEqual([
			'sh',
			'-c',
			'curl -f http://x || exit 1',
		])
		expect(parseCommandLine('echo "dice \\"hola\\"" a\\ b')).toEqual([
			'echo',
			'dice "hola"',
			'a b',
		])
		expect(parseCommandLine('')).toEqual([])
		expect(parseCommandLine("''")).toEqual([''])
	})

	it('no expande nada: $ y los operadores quedan como texto', () => {
		expect(parseCommandLine('echo $HOME && ls')).toEqual(['echo', '$HOME', '&&', 'ls'])
	})

	it('una comilla sin cerrar es null', () => {
		expect(parseCommandLine("sh -c 'sin cerrar")).toBeNull()
		expect(parseCommandLine('echo "a')).toBeNull()
	})

	it('formatear y volver a leer da la misma lista', () => {
		const cases: string[][] = [
			['redis-cli', 'ping'],
			['sh', '-c', 'curl -f http://127.0.0.1:3000/health || exit 1'],
			['echo', "it's", 'a "quote"', 'back\\slash', ''],
			['--name', 'con  dos espacios'],
		]
		for (const args of cases) {
			expect(parseCommandLine(formatCommandLine(args))).toEqual(args)
		}
		expect(formatCommandLine(['node', 'dist/x.js'])).toBe('node dist/x.js')
		expect(formatCommandLine(['sh', '-c', 'a b'])).toBe("sh -c 'a b'")
	})
})
