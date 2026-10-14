import { describe, expect, it } from 'vitest'
import {
	InvalidRegistryHost,
	parseRegistryCredentials,
	parseRegistryHost,
	RequiredField,
} from '../..'

describe('registryHost', () => {
	it('acepta hosts de registry, con puerto', () => {
		expect(parseRegistryHost('ghcr.io')).toBe('ghcr.io')
		expect(parseRegistryHost('Registry.Example.com:5000')).toBe('registry.example.com:5000')
		expect(parseRegistryHost('localhost:5000')).toBe('localhost:5000')
	})

	it('limpia lo que se pega desde el navegador', () => {
		expect(parseRegistryHost(' https://ghcr.io/ ')).toBe('ghcr.io')
	})

	it('rechaza rutas, espacios y guiones en los bordes', () => {
		for (const bad of ['', 'ghcr.io/cpu-commits', 'mi registry', '-ghcr.io', 'ghcr.io:']) {
			expect(() => parseRegistryHost(bad)).toThrow(InvalidRegistryHost)
		}
	})

	it('usuario y contraseña son obligatorios; la contraseña no se recorta', () => {
		expect(parseRegistryCredentials({ username: ' bot ', password: ' x ' })).toEqual({
			username: 'bot',
			password: ' x ',
		})
		expect(() => parseRegistryCredentials({ username: ' ', password: 'x' })).toThrow(
			RequiredField,
		)
		expect(() => parseRegistryCredentials({ username: 'bot', password: '' })).toThrow(
			RequiredField,
		)
	})
})
