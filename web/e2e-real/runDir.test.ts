import { existsSync, mkdtempSync, readdirSync, rmSync, statSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { createRunDir } from './runDir'

// La carpeta temporal de una corrida del e2e real (globalSetup.ts): la clave maestra, la base
// y el log. Se crea antes de que haya una limpieza que la borre, así que si algo falla al
// prepararla la borra ella misma.
describe('createRunDir', () => {
	let root: string

	beforeEach(() => {
		root = mkdtempSync(join(tmpdir(), 'muelle-it7-rundir-'))
	})

	afterEach(() => {
		rmSync(root, { recursive: true, force: true })
	})

	it('crea la carpeta con la clave maestra en 0600 y dice dónde va cada cosa', () => {
		const run = createRunDir(root, () => Buffer.from('clave\n'))
		expect(run.logPath).toBe(join(run.dir, 'muelle.log'))
		expect(run.keyFile).toBe(join(run.dir, 'master.key'))
		expect(run.dataDir).toBe(join(run.dir, 'data'))
		expect(statSync(run.keyFile).mode & 0o777).toBe(0o600)
	})

	it('si genkey falla, no deja una carpeta vacía', () => {
		expect(() =>
			createRunDir(root, () => {
				throw new Error('genkey: no se pudo leer /dev/urandom')
			}),
		).toThrow('genkey')
		expect(readdirSync(root)).toEqual([])
		expect(existsSync(root)).toBe(true)
	})
})
