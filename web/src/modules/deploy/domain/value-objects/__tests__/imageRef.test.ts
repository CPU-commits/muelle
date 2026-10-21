import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import {
	imageName,
	imageTag,
	isValidImageOverride,
	isValidImageRef,
	parseImageOverride,
	shortDigest,
	splitImageRef,
} from '../imageRef'
import oracle from './imageRefOracle.json'

const DIGEST = `sha256:9b2de1f4${'0'.repeat(48)}5d17c07a`

describe('splitImageRef', () => {
	it('separa la ref del digest', () => {
		expect(splitImageRef(`ghcr.io/aumi-app/backend-app:sha-3f9c2e1@${DIGEST}`)).toEqual({
			ref: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
			digest: DIGEST,
		})
	})

	it('sin @ todavía no tiene digest', () => {
		expect(splitImageRef('nats:2.10-alpine')).toEqual({ ref: 'nats:2.10-alpine', digest: null })
	})
})

// La regla es la del backend (validate.ImageRef, de la Fase 2): lo que uno acepta y el
// otro no es un 422 que el panel podría haber evitado, o un deploy que el panel frena sin
// razón. imageRefOracle.json salió de correr el código de Go con estas entradas.
describe('isValidImageRef: igual que el backend', () => {
	it.each(oracle.cases.map((entry) => [entry.input, entry.valid] as const))(
		'%j → %s',
		(input, valid) => {
			expect(isValidImageRef(input)).toBe(valid)
		},
	)

	it.each([
		['Ghcr.io/Foo/Bar:Latest', false],
		['a//b', false],
		['a::b', false],
		['a:b:c', false],
		['a/../b', false],
		['a', true],
		['nginx:1.0-', true],
		// El host puede llevar mayúsculas; el repositorio no.
		['Ghcr.io/foo/bar:Latest', true],
		['ghcr.io/Foo/bar', false],
		// El digest va en hexa minúscula, con el largo de su algoritmo.
		[`redis@sha256:${'a'.repeat(64)}`, true],
		[`redis@sha256:${'A'.repeat(64)}`, false],
		[`redis@sha256:${'a'.repeat(63)}`, false],
		// Un id de 64 hexa suelto no es un nombre.
		['a'.repeat(64), false],
		[`${'a'.repeat(64)}:tag`, true],
		// La defensa extra del backend.
		['--help', false],
		['${X}', false],
		['a'.repeat(513), false],
	] as const)('%j → %s', (input, valid) => {
		expect(isValidImageRef(input)).toBe(valid)
	})
})

describe('parseImageOverride', () => {
	it('recorta espacios y devuelve la referencia', () => {
		expect(parseImageOverride('  ghcr.io/aumi-app/backend-app:sha-3f9c2e1 ')).toBe(
			'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		)
	})

	it.each(['', '   ', 'con espacio:tag', 'app:tag;rm -rf /', '-app:tag', 'app:', '$(id)'])(
		'rechaza %j',
		(raw) => {
			expect(() => parseImageOverride(raw, 'web')).toThrow(ValidationError)
			expect(isValidImageOverride(raw)).toBe(false)
		},
	)

	it('el error lleva el campo pedido', () => {
		try {
			parseImageOverride('a b', 'web')
			expect.unreachable()
		} catch (error) {
			expect(error).toBeInstanceOf(ValidationError)
			expect((error as ValidationError).field).toBe('web')
		}
	})
})

describe('nombres cortos', () => {
	it('imageName se queda con lo que va después del último /', () => {
		expect(imageName('ghcr.io/aumi-app/backend-app:sha-e52b8f0')).toBe(
			'backend-app:sha-e52b8f0',
		)
		expect(imageName('redis:7.4-alpine')).toBe('redis:7.4-alpine')
	})

	it('imageTag es solo el tag, o el nombre si no tiene', () => {
		expect(imageTag('ghcr.io/aumi-app/backend-app:sha-3f9c2e1')).toBe('sha-3f9c2e1')
		expect(imageTag('localhost:5000/org/app')).toBe('app')
	})

	it('shortDigest deja los primeros y los últimos 8', () => {
		expect(shortDigest(DIGEST)).toBe('sha256:9b2de1f4…5d17c07a')
		expect(shortDigest('sha256:abc')).toBe('sha256:abc')
	})
})
