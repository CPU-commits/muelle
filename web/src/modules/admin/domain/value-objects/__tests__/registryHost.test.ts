import { describe, expect, it } from 'vitest'
import { InvalidRegistryHost, parseRegistryHost, type RegistryHostProblem } from '../..'

function problemOf(raw: string): RegistryHostProblem | 'ok' | 'other' {
	try {
		parseRegistryHost(raw)
		return 'ok'
	} catch (error) {
		return error instanceof InvalidRegistryHost ? error.problem : 'other'
	}
}

// El backend exige el host en su forma canónica y no lo normaliza (contrato §2.10).
describe('registryHost', () => {
	it('acepta hosts canónicos, con puerto', () => {
		expect(parseRegistryHost('ghcr.io')).toBe('ghcr.io')
		expect(parseRegistryHost('registry.example.com:5000')).toBe('registry.example.com:5000')
		expect(parseRegistryHost('localhost:5000')).toBe('localhost:5000')
		expect(parseRegistryHost('docker.io')).toBe('docker.io')
	})

	it('solo arregla lo que no cambia de host: espacios y mayúsculas', () => {
		expect(parseRegistryHost(' GHCR.io ')).toBe('ghcr.io')
	})

	it('los alias de Docker Hub pasan a docker.io (con un alias la credencial no se usaría)', () => {
		for (const alias of [
			'index.docker.io',
			'registry-1.docker.io',
			'registry.hub.docker.com',
		]) {
			expect(parseRegistryHost(alias)).toBe('docker.io')
		}
	})

	it('rechaza esquema, ruta y "/" final con un motivo claro', () => {
		expect(problemOf('https://ghcr.io')).toBe('scheme')
		expect(problemOf('http://registry.example.com:5000')).toBe('scheme')
		expect(problemOf('ghcr.io/cpu-commits')).toBe('path')
		expect(problemOf('ghcr.io/')).toBe('path')
	})

	it('rechaza lo que no es un host', () => {
		expect(problemOf('')).toBe('empty')
		for (const bad of [
			'mi registry',
			'-ghcr.io',
			'ghcr.io:',
			'ghcr..io',
			'ghcr_io',
			'ghcr.io:99999',
		]) {
			expect(problemOf(bad)).toBe('format')
		}
	})
})
