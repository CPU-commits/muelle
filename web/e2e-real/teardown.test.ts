import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { cleanUpRun, type RunFiles } from './teardown'

// La limpieza de globalSetup.ts, ya con muelle apagado: la carpeta temporal de la corrida
// tiene la clave maestra y la base, así que se borra pase lo que pase con Docker.
describe('cleanUpRun', () => {
	let root: string
	let files: RunFiles

	beforeEach(() => {
		root = mkdtempSync(join(tmpdir(), 'muelle-it7-teardown-'))
		const dir = join(root, 'run')
		files = {
			dir,
			logPath: join(dir, 'muelle.log'),
			logCopy: join(root, 'test-results', 'real-muelle.log'),
		}
		mkdirSync(join(dir, 'data'), { recursive: true })
		writeFileSync(join(dir, 'master.key'), 'clave', { mode: 0o600 })
		writeFileSync(join(dir, 'data', 'muelle.db'), 'base')
		writeFileSync(files.logPath, '{"level":"INFO","msg":"listening"}\n')
	})

	afterEach(() => {
		rmSync(root, { recursive: true, force: true })
	})

	it('copia el log y borra la carpeta si Docker queda limpio', async () => {
		await cleanUpRun(files, () => Promise.resolve([]), 'it7pabcd')
		expect(existsSync(files.dir)).toBe(false)
		expect(readFileSync(files.logCopy, 'utf8')).toContain('listening')
	})

	it('copia el log y borra la carpeta aunque falle el CLI de docker, y después falla', async () => {
		const docker = () => Promise.reject(new Error('docker stack ls: Cannot connect'))
		await expect(cleanUpRun(files, docker, 'it7pabcd')).rejects.toThrow(/docker stack ls/)
		expect(existsSync(files.dir)).toBe(false)
		expect(readFileSync(files.logCopy, 'utf8')).toContain('listening')
	})

	it('nombra lo que quedó de Docker, después de borrar la carpeta', async () => {
		const docker = () => Promise.resolve(['it7pabcd_web', 'it7pabcd_web_default'])
		await expect(cleanUpRun(files, docker, 'it7pabcd')).rejects.toThrow(
			'no se pudo borrar lo de it7pabcd: it7pabcd_web, it7pabcd_web_default',
		)
		expect(existsSync(files.dir)).toBe(false)
	})

	it('sin log (muelle no llegó a escribir), borra la carpeta igual', async () => {
		rmSync(files.logPath)
		await cleanUpRun(files, () => Promise.resolve([]), 'it7pabcd')
		expect(existsSync(files.dir)).toBe(false)
		expect(existsSync(files.logCopy)).toBe(false)
	})
})
