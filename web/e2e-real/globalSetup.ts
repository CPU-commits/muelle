import { type ChildProcess, execFileSync, spawn } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { createWriteStream, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { createInterface } from 'node:readline'
import { realCaddy, removeOwned, swarmManager, sweepStale } from './docker'
import { PREFIX, publishRun, REAL_PORT, REAL_URL } from './env'
import { createRunDir } from './runDir'
import { cleanUpRun } from './teardown'

// Arranca el `muelle serve` del recorrido (journey.real.ts) con una base nueva y devuelve la
// limpieza, que Playwright corre al final aunque los tests fallen: para muelle, borra todo
// lo de Docker del proyecto de la corrida (it7p<id>), guarda el log de muelle en
// test-results/real-muelle.log y borra la carpeta temporal (teardown.ts: el log y la carpeta,
// aunque falle el CLI de docker).
//
// El binario lo compila scripts/e2e-real.mjs (con -tags panel y el build de web/) y llega
// en MUELLE_E2E_BIN. El entorno de muelle es el de desarrollo por http: MUELLE_PANEL_ORIGIN
// en 127.0.0.1 (no es un dominio: no hay dominio del panel y el proxy queda deshabilitado,
// sin muelle-caddy) y MUELLE_SECURE_COOKIES=false, que settings solo acepta con un origen
// http. Ninguna otra MUELLE_* del entorno pasa.

const START_TIMEOUT_MS = 60_000
const LOG_COPY = resolve('test-results/real-muelle.log')

function log(line: string): void {
	console.log(`[e2e-real] ${line}`)
}

interface Started {
	setupUrl: string
}

/** Espera las líneas setup_url y listening del log JSON de serve (main.go). */
function waitStarted(child: ChildProcess, logPath: string): Promise<Started> {
	return new Promise((done, fail) => {
		let setupUrl: string | null = null
		const timer = setTimeout(
			() =>
				fail(
					new Error(
						`muelle no arrancó en ${START_TIMEOUT_MS / 1000} s (log: ${logPath})`,
					),
				),
			START_TIMEOUT_MS,
		)
		const finish = (result: Error | Started) => {
			clearTimeout(timer)
			child.off('exit', onExit)
			if (result instanceof Error) fail(result)
			else done(result)
		}
		const onExit = (code: number | null) =>
			finish(new Error(`muelle terminó al arrancar (código ${code}); log: ${logPath}`))
		child.on('exit', onExit)
		if (!child.stdout) return finish(new Error('muelle sin stdout'))
		createInterface({ input: child.stdout }).on('line', (line) => {
			let entry: { event?: string; url?: string }
			try {
				entry = JSON.parse(line) as typeof entry
			} catch {
				return
			}
			if (entry.event === 'setup_url' && entry.url) setupUrl = entry.url
			if (entry.event === 'listening') {
				if (!setupUrl)
					finish(
						new Error(
							`muelle arrancó sin link de setup (¿la base no es nueva?); log: ${logPath}`,
						),
					)
				else finish({ setupUrl })
			}
		})
	})
}

/** Apaga muelle (SIGTERM, y SIGKILL a los 15 s) y espera a que cierre su salida. */
async function stop(child: ChildProcess, closed: Promise<unknown>): Promise<void> {
	if (child.exitCode === null && child.signalCode === null) child.kill('SIGTERM')
	const timer = setTimeout(() => child.kill('SIGKILL'), 15_000)
	await closed
	clearTimeout(timer)
}

export default async function globalSetup(): Promise<() => Promise<void>> {
	const bin = process.env.MUELLE_E2E_BIN
	if (!bin || !existsSync(bin)) {
		throw new Error(
			'falta MUELLE_E2E_BIN (el muelle con -tags panel): corre pnpm test:e2e:real',
		)
	}
	if (!swarmManager()) {
		throw new Error(
			'el Docker local no es manager de un Swarm activo: el e2e real necesita uno (no lo crea)',
		)
	}
	if (realCaddy()) {
		throw new Error(
			'hay un muelle-caddy en este Docker: el e2e real lo conectaría a la red de su stage. Córrelo en otro nodo',
		)
	}
	await sweepStale(log)

	const id = randomBytes(2).toString('hex')
	const project = `${PREFIX}p${id}`
	// Si genkey falla, createRunDir borra la carpeta: la limpieza de abajo todavía no existe.
	const { dir, logPath, keyFile, dataDir } = createRunDir(tmpdir(), () =>
		execFileSync(bin, ['genkey']),
	)

	const env: NodeJS.ProcessEnv = {}
	for (const [key, value] of Object.entries(process.env)) {
		if (!key.startsWith('MUELLE_')) env[key] = value
	}
	Object.assign(env, {
		MUELLE_LISTEN: `127.0.0.1:${REAL_PORT}`,
		MUELLE_PANEL_ORIGIN: REAL_URL,
		MUELLE_SECURE_COOKIES: 'false',
		MUELLE_DATA_DIR: dataDir,
		MUELLE_MASTER_KEY_FILE: keyFile,
	})
	const child = spawn(bin, ['serve'], { env, stdio: ['ignore', 'pipe', 'pipe'] })
	const childClosed = new Promise<void>((done) => child.once('close', () => done()))
	// Las dos salidas al mismo archivo: ninguna lo cierra, se cierra cuando muelle terminó.
	const out = createWriteStream(logPath)
	const closed = new Promise<void>((done) => out.once('close', () => done()))
	child.stdout.pipe(out, { end: false })
	child.stderr.pipe(out, { end: false })

	const teardown = async (): Promise<void> => {
		try {
			await stop(child, childClosed)
			out.end()
			await closed
		} finally {
			log(`log de muelle: ${LOG_COPY}`)
			await cleanUpRun(
				{ dir, logPath, logCopy: LOG_COPY },
				() => removeOwned(project),
				project,
			)
		}
	}

	try {
		const { setupUrl } = await waitStarted(child, logPath)
		if (!setupUrl.startsWith(`${REAL_URL}/setup?token=`)) {
			throw new Error(`el link de setup no es de ${REAL_URL}: ${setupUrl}`)
		}
		publishRun({ id, project, username: `${PREFIX}admin`, setupUrl })
		log(`muelle en ${REAL_URL}, proyecto ${project}`)
	} catch (error) {
		await teardown().catch((cleanup: unknown) => log(String(cleanup)))
		throw error
	}
	return teardown
}
