import { describe, expect, it } from 'vitest'
import {
	COMPOSE_MAX_BYTES,
	composeProblem,
	InvalidCompose,
	parseComposeYaml,
	summarizeCompose,
} from '../..'

const AGENTS = `services:
  gateway:
    image: ghcr.io/aumi-app/agents-gateway:staging
    environment:
      LLM_API_KEY: \${LLM_API_KEY}
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/health"]
      interval: 10s
  runner:
    image: "ghcr.io/aumi-app/agents-runner:staging" # el que corre los trabajos
    deploy:
      replicas: 2
    volumes:
      - workspace:/workspace
      - ./local:/local
      - type: volume
        source: cache
        target: /cache
  scheduler:
    image: ghcr.io/aumi-app/agents-scheduler:staging
    healthcheck:
      disable: true

volumes:
  workspace: {}
  cache: {}
`

describe('composeYaml', () => {
	it('exige un compose no vacío de hasta 256 KiB (en bytes)', () => {
		expect(composeProblem('  \n ')).toBe('empty')
		expect(composeProblem('a'.repeat(COMPOSE_MAX_BYTES))).toBeNull()
		expect(composeProblem('a'.repeat(COMPOSE_MAX_BYTES + 1))).toBe('too_large')
		// "é" pesa 2 bytes: la mitad de caracteres ya llega al límite.
		expect(composeProblem('é'.repeat(COMPOSE_MAX_BYTES / 2 + 1))).toBe('too_large')
		expect(() => parseComposeYaml('')).toThrow(InvalidCompose)
		expect(parseComposeYaml(AGENTS)).toBe(AGENTS)
	})

	it('resume los servicios: imagen, réplicas, healthcheck y volúmenes nombrados', () => {
		expect(summarizeCompose(AGENTS)).toEqual([
			{
				name: 'gateway',
				line: 2,
				duplicate: false,
				image: 'ghcr.io/aumi-app/agents-gateway:staging',
				replicas: null,
				healthcheck: 'defined',
				volumes: [],
			},
			{
				name: 'runner',
				line: 9,
				duplicate: false,
				image: 'ghcr.io/aumi-app/agents-runner:staging',
				replicas: 2,
				healthcheck: 'missing',
				volumes: ['workspace', 'cache'],
			},
			{
				name: 'scheduler',
				line: 19,
				duplicate: false,
				image: 'ghcr.io/aumi-app/agents-scheduler:staging',
				replicas: null,
				healthcheck: 'disabled',
				volumes: [],
			},
		])
	})

	it('entiende la indentación de 4, los comentarios y la sintaxis en línea', () => {
		const yaml = [
			'# comentario',
			'version: "3.9"',
			'services:',
			'    web: { image: nginx:1.27 }',
			'    "api":',
			'        image: x',
			'        deploy: { replicas: 3 }',
			'        volumes: [data:/data, /host:/host]',
			'networks: {}',
		].join('\n')
		expect(summarizeCompose(yaml)).toEqual([
			{
				name: 'web',
				line: 4,
				duplicate: false,
				image: 'nginx:1.27',
				replicas: null,
				healthcheck: 'missing',
				volumes: [],
			},
			{
				name: 'api',
				line: 5,
				duplicate: false,
				image: 'x',
				replicas: 3,
				healthcheck: 'missing',
				volumes: ['data'],
			},
		])
	})

	it('un servicio repetido queda marcado desde la segunda vez, con su línea', () => {
		const yaml =
			'services:\n  web:\n    image: a\n  web:\n    image: b\n  worker:\n    image: c\n'

		const services = summarizeCompose(yaml)

		expect(services.map(({ name, line, duplicate }) => ({ name, line, duplicate }))).toEqual([
			{ name: 'web', line: 2, duplicate: false },
			{ name: 'web', line: 4, duplicate: true },
			{ name: 'worker', line: 6, duplicate: false },
		])
	})

	it('sin `services:` en bloque no hay resumen', () => {
		expect(summarizeCompose('services: {web: {image: nginx}}')).toEqual([])
		expect(summarizeCompose('')).toEqual([])
	})
})
