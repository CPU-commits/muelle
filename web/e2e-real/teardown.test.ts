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

	it('falla si muelle registró un error interno (un 500), después de limpiar', async () => {
		const internal =
			'{"level":"ERROR","msg":"error interno","method":"GET","path":"/api/v1/projects","err":"falló la base"}'
		writeFileSync(
			files.logPath,
			[
				'{"level":"INFO","msg":"listening"}',
				'{"level":"ERROR","msg":"tarea periódica","job":"dns","err":"sin red"}',
				internal,
				'no es JSON',
			].join('\n'),
		)
		await expect(cleanUpRun(files, () => Promise.resolve([]), 'it7pabcd')).rejects.toThrow(
			internal,
		)
		expect(existsSync(files.dir)).toBe(false)
		expect(readFileSync(files.logCopy, 'utf8')).toContain(internal)
	})

	it('también falla con un error interno del despachador de deploys (no es un 500)', async () => {
		// dispatcher.go registra "deploy: error interno" con app y deploy, sin el texto.
		const internal = '{"level":"ERROR","msg":"deploy: error interno","app":3,"deploy":7}'
		writeFileSync(
			files.logPath,
			['{"level":"INFO","msg":"deploy cerrado","app":3,"deploy":7}', internal].join('\n'),
		)
		await expect(cleanUpRun(files, () => Promise.resolve([]), 'it7pabcd')).rejects.toThrow(
			internal,
		)
		expect(existsSync(files.dir)).toBe(false)
	})

	it('con restos de Docker y errores internos, un solo error que nombra los dos', async () => {
		const internal = '{"level":"ERROR","msg":"error interno","path":"/api/v1/projects"}'
		writeFileSync(files.logPath, `${internal}\n`)
		const docker = () => Promise.resolve(['it7pabcd_web'])
		const error = await cleanUpRun(files, docker, 'it7pabcd').catch((caught: unknown) => caught)
		expect(error).toBeInstanceOf(Error)
		const message = (error as Error).message
		expect(message).toContain('no se pudo borrar lo de it7pabcd: it7pabcd_web')
		expect(message).toContain(internal)
		expect(existsSync(files.dir)).toBe(false)
	})

	it('si falla el CLI de docker, el error también nombra los errores internos', async () => {
		const internal = '{"level":"ERROR","msg":"error interno","path":"/api/v1/projects"}'
		writeFileSync(files.logPath, `${internal}\n`)
		const docker = () => Promise.reject(new Error('docker stack ls: Cannot connect'))
		const error = await cleanUpRun(files, docker, 'it7pabcd').catch((caught: unknown) => caught)
		const message = (error as Error).message
		expect(message).toContain('docker stack ls: Cannot connect')
		expect(message).toContain(internal)
		expect(existsSync(files.dir)).toBe(false)
	})

	it('sin log (muelle no llegó a escribir), borra la carpeta igual', async () => {
		rmSync(files.logPath)
		await cleanUpRun(files, () => Promise.resolve([]), 'it7pabcd')
		expect(existsSync(files.dir)).toBe(false)
		expect(existsSync(files.logCopy)).toBe(false)
	})
})
