import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import {
	imageName,
	imageTag,
	isValidImageOverride,
	parseImageOverride,
	shortDigest,
	splitImageRef,
} from '../imageRef'

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

describe('parseImageOverride', () => {
	it.each([
		'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		'nats:2.10-alpine',
		'localhost:5000/org/app:v1',
		`redis@${'sha256:' + 'a'.repeat(64)}`,
	])('acepta %s', (raw) => {
		expect(parseImageOverride(`  ${raw} `)).toBe(raw)
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
