import { describe, expect, it } from 'vitest'
import { MAX_VALUE_BYTES } from '../../value-objects/envLimits'
import { parseDotenv } from '../dotenv'

const values = (text: string) =>
	Object.fromEntries(parseDotenv(text).entries.map((entry) => [entry.key, entry.value]))

describe('parseDotenv', () => {
	it('lee KEY=VALUE e ignora comentarios y líneas en blanco', () => {
		const parsed = parseDotenv(
			['# base de datos', '', 'DATABASE_URL=postgres://db:5432/app', '   ', 'PORT=3000'].join(
				'\n',
			),
		)
		expect(parsed.issues).toEqual([])
		expect(parsed.entries).toEqual([
			{ key: 'DATABASE_URL', value: 'postgres://db:5432/app', line: 3 },
			{ key: 'PORT', value: '3000', line: 5 },
		])
	})

	it('ignora el prefijo export y los espacios alrededor de la clave y del =', () => {
		expect(values('export TZ=UTC\nexport\tLANG=es\n  PORT = 3000  \nexport=1')).toEqual({
			TZ: 'UTC',
			LANG: 'es',
			PORT: '3000',
			export: '1',
		})
	})

	it('un valor vacío es una variable vacía', () => {
		expect(values('EMPTY=\nEMPTY_QUOTED=""\nEMPTY_COMMENT= # nada')).toEqual({
			EMPTY: '',
			EMPTY_QUOTED: '',
			EMPTY_COMMENT: '',
		})
	})

	it('un # empieza un comentario solo con un espacio antes', () => {
		expect(
			values('URL=https://x.cl/#ancla\nLOG=info # comentario\nHASH=#abc\nTAB=a\t#c'),
		).toEqual({
			URL: 'https://x.cl/#ancla',
			LOG: 'info',
			HASH: '#abc',
			TAB: 'a',
		})
	})

	it('comillas dobles: traduce \\n \\t \\" \\\\ y deja el resto', () => {
		expect(
			values('A="Hola \\"mundo\\"\\n\\tfin\\\\" # c\nB="un # no es comentario"\nC="\\x"'),
		).toEqual({
			A: 'Hola "mundo"\n\tfin\\',
			B: 'un # no es comentario',
			C: '\\x',
		})
	})

	it('comillas simples: literal, sin escapes', () => {
		expect(values("RAW='sin \\n escapes ${HOME}'")).toEqual({ RAW: 'sin \\n escapes ${HOME}' })
	})

	it('no expande ${VAR}', () => {
		expect(values('URL=${HOST}:${PORT}')).toEqual({ URL: '${HOST}:${PORT}' })
	})

	it('las comillas pueden abarcar varias líneas, y la entrada recuerda dónde empieza', () => {
		const text = ['A=1', 'KEY="-----BEGIN-----', 'abc', '-----END-----"', 'B=2'].join('\n')
		const parsed = parseDotenv(text)
		expect(parsed.issues).toEqual([])
		expect(parsed.entries).toEqual([
			{ key: 'A', value: '1', line: 1 },
			{ key: 'KEY', value: '-----BEGIN-----\nabc\n-----END-----', line: 2 },
			{ key: 'B', value: '2', line: 5 },
		])
	})

	it('respeta los espacios dentro de las comillas, también al final de una línea', () => {
		expect(values('A="  dos  \nlíneas  "')).toEqual({ A: '  dos  \nlíneas  ' })
	})

	it('acepta finales de línea de Windows y un BOM al principio', () => {
		expect(values('\uFEFFA=1\r\nB="x\r\ny"\r\n')).toEqual({ A: '1', B: 'x\ny' })
	})

	it('si una clave se repite gana la última y lo avisa', () => {
		const parsed = parseDotenv('A=1\nB=2\nA=3')
		expect(parsed.entries).toEqual([
			{ key: 'B', value: '2', line: 2 },
			{ key: 'A', value: '3', line: 3 },
		])
		expect(parsed.duplicates).toEqual([{ key: 'A', line: 3, firstLine: 1 }])
	})

	it('valida las claves como el backend, con el número de línea', () => {
		const parsed = parseDotenv(
			[
				'OK=1',
				'COMPOSE_PROFILES=debug',
				'1BAD=x',
				'MY-VAR=x',
				`${'A'.repeat(129)}=x`,
				'=x',
			].join('\n'),
		)
		expect(parsed.entries.map((entry) => entry.key)).toEqual(['OK'])
		expect(parsed.issues).toEqual([
			{ line: 2, problem: 'reserved_key', key: 'COMPOSE_PROFILES' },
			{ line: 3, problem: 'invalid_key', key: '1BAD' },
			{ line: 4, problem: 'invalid_key', key: 'MY-VAR' },
			{ line: 5, problem: 'key_too_long', key: 'A'.repeat(129) },
			{ line: 6, problem: 'empty_key' },
		])
	})

	it('una línea sin = es un error', () => {
		expect(parseDotenv('A=1\nesto no es una variable\nexport B').issues).toEqual([
			{ line: 2, problem: 'missing_equals' },
			{ line: 3, problem: 'missing_equals' },
		])
	})

	it('comillas sin cerrar y texto después de cerrarlas', () => {
		expect(parseDotenv('A="abierta\nB=2').issues).toEqual([
			{ line: 1, problem: 'unterminated_quote', key: 'A' },
		])
		expect(parseDotenv("A='x' y").issues).toEqual([
			{ line: 1, problem: 'trailing_text', key: 'A' },
		])
	})

	it('valida los valores: 64 KiB, NUL y UTF-8', () => {
		const parsed = parseDotenv(`BIG=${'a'.repeat(MAX_VALUE_BYTES + 1)}\nNUL="a\0b"\nBAD=\uD800`)
		expect(parsed.issues).toEqual([
			{ line: 1, problem: 'value_too_large', key: 'BIG' },
			{ line: 2, problem: 'value_nul', key: 'NUL' },
			{ line: 3, problem: 'value_invalid_utf8', key: 'BAD' },
		])
	})

	it('un texto vacío no tiene nada', () => {
		const nothing = { entries: [], issues: [], duplicates: [], swallowed: [] }
		expect(parseDotenv('')).toEqual(nothing)
		expect(parseDotenv('\n\n# solo comentarios\n')).toEqual(nothing)
	})

	it('unas comillas sin cerrar se marcan en la línea donde abren, y avisa las variables que se tragaron', () => {
		const parsed = parseDotenv('A=1\nPEM="-----BEGIN-----\nB=2\nexport C=3\nabc==')

		expect(parsed.issues).toEqual([{ line: 2, problem: 'unterminated_quote', key: 'PEM' }])
		expect(parsed.swallowed).toEqual([
			{ line: 3, key: 'PEM', looksLike: 'B' },
			{ line: 4, key: 'PEM', looksLike: 'C' },
		])
		expect(parsed.entries.map((entry) => entry.key)).toEqual(['A'])
	})

	it('un valor de varias líneas que cierra bien también avisa las líneas con forma de KEY=', () => {
		const parsed = parseDotenv('MSG="hola\nNEXT=2\nchau"\nB=1')

		expect(parsed.issues).toEqual([])
		expect(values('MSG="hola\nNEXT=2\nchau"')).toEqual({ MSG: 'hola\nNEXT=2\nchau' })
		expect(parsed.swallowed).toEqual([{ line: 2, key: 'MSG', looksLike: 'NEXT' }])
		// El relleno de base64 de una clave PEM no parece una variable.
		expect(parseDotenv('PEM="-----BEGIN-----\nQUJD==\n-----END-----"').swallowed).toEqual([])
	})

	it('en comillas dobles, \\$ es un $ (como compose y godotenv)', () => {
		expect(values('A="cuesta \\$5 y \\${HOME}"')).toEqual({ A: 'cuesta $5 y ${HOME}' })
		expect(values("B='\\$literal'")).toEqual({ B: '\\$literal' })
	})

	it('solo recorta espacios ASCII: un espacio de no separación es parte del valor', () => {
		const nbsp = String.fromCharCode(0xa0)

		expect(values(`A=${nbsp}x${nbsp}`)).toEqual({ A: `${nbsp}x${nbsp}` })
		expect(values(`B=x${nbsp}#no-es-comentario`)).toEqual({ B: `x${nbsp}#no-es-comentario` })
		expect(values('C= \t x \t ')).toEqual({ C: 'x' })
		expect(parseDotenv(`${nbsp}D=1`).issues).toEqual([
			{ line: 1, problem: 'invalid_key', key: `${nbsp}D` },
		])
		expect(parseDotenv(`E="x"${nbsp}`).issues).toEqual([
			{ line: 1, problem: 'trailing_text', key: 'E' },
		])
	})
})
