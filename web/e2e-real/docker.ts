import { spawnSync } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

// El CLI de docker para lo que el e2e real mira o limpia por su cuenta. Solo toca nombres de
// un proyecto it7p<id> (owned): stacks, servicios y redes son <proyecto>_…, y los volúmenes
// de un stack, <stack>_…. Nunca hace swarm init/leave ni toca las piezas fijas de muelle.

/** Las piezas fijas de muelle que un `muelle serve` de prueba podría tocar (ver realCaddy). */
const REAL_CADDY = 'muelle-caddy'
const STACK_LABEL = 'com.docker.stack.namespace'
/** Lo de una corrida anterior que no limpió: más viejo que esto (el timeout del recorrido es de 10 min). */
const STALE_AFTER_MS = 30 * 60_000
const STALE_PROJECT = /^it7p[0-9a-f]{4}(?=_|$)/

function run(args: string[]): { ok: boolean; out: string; err: string } {
	const result = spawnSync('docker', args, { encoding: 'utf8', timeout: 120_000 })
	return { ok: result.status === 0, out: result.stdout ?? '', err: result.stderr ?? '' }
}

function docker(args: string[]): string {
	const result = run(args)
	if (!result.ok) throw new Error(`docker ${args.join(' ')}: ${result.err.trim()}`)
	return result.out
}

function lines(out: string): string[] {
	return out
		.split('\n')
		.map((line) => line.trim())
		.filter(Boolean)
}

/** Si el Docker local es manager de un Swarm activo (lo que necesita un deploy). */
export function swarmManager(): boolean {
	const info = run(['info', '--format', '{{.Swarm.LocalNodeState}} {{.Swarm.ControlAvailable}}'])
	return info.ok && info.out.trim() === 'active true'
}

/**
 * Si existe el muelle-caddy de una instalación de verdad. Crear un stage conecta en caliente
 * muelle-caddy a la red del stage, y borrarlo lo desconecta (ingress, EnsureStageNetwork),
 * aunque el proxy esté deshabilitado: con uno de verdad en este Docker, el e2e no corre.
 */
export function realCaddy(): boolean {
	return run(['container', 'inspect', REAL_CADDY]).ok
}

/** `name` es de `prefix` (it7p<id>): el mismo nombre, o uno que sigue con "_". */
export function owned(name: string, prefix: string): boolean {
	return name === prefix || name.startsWith(`${prefix}_`)
}

/** Si `nginx:alpine` (o la imagen que sea) ya está en el nodo. */
export function hasImage(image: string): boolean {
	return run(['image', 'inspect', image]).ok
}

function list(kind: 'stack' | 'service' | 'network' | 'volume'): string[] {
	const args = kind === 'stack' ? ['stack', 'ls'] : [kind, 'ls']
	return lines(docker([...args, '--format', '{{.Name}}']))
}

function stackContainers(prefix: string): string[] {
	const out = docker([
		'ps',
		'-a',
		'--filter',
		`label=${STACK_LABEL}`,
		'--format',
		`{{.Label "${STACK_LABEL}"}}`,
	])
	return lines(out).filter((stack) => owned(stack, prefix))
}

async function until(timeoutMs: number, done: () => boolean): Promise<boolean> {
	const end = Date.now() + timeoutMs
	for (;;) {
		if (done()) return true
		if (Date.now() > end) return false
		await sleep(2_000)
	}
}

/**
 * Borra todo lo de `prefix` (un it7p<id>): los stacks y servicios, espera a que Swarm baje
 * sus contenedores, y borra las redes (reintentando mientras siguen en uso) y los volúmenes.
 * Devuelve lo que no pudo borrar.
 */
export async function removeOwned(prefix: string): Promise<string[]> {
	if (!STALE_PROJECT.test(prefix))
		throw new Error(`removeOwned: "${prefix}" no es un proyecto it7p<id>`)
	for (const stack of list('stack').filter((name) => owned(name, prefix))) {
		run(['stack', 'rm', stack])
	}
	for (const service of list('service').filter((name) => owned(name, prefix))) {
		run(['service', 'rm', service])
	}
	await until(90_000, () => stackContainers(prefix).length === 0)

	const networks = () => list('network').filter((name) => owned(name, prefix))
	await until(60_000, () => {
		for (const network of networks()) run(['network', 'rm', network])
		return networks().length === 0
	})
	for (const volume of list('volume').filter((name) => owned(name, prefix))) {
		run(['volume', 'rm', volume])
	}

	return [
		...list('service').filter((name) => owned(name, prefix)),
		...stackContainers(prefix).map((stack) => `contenedores de ${stack}`),
		...networks(),
		...list('volume').filter((name) => owned(name, prefix)),
	]
}

function createdAt(kind: 'service' | 'network' | 'volume', name: string): number | null {
	// En JSON, las fechas salen en RFC 3339 (con zona), que Date.parse entiende.
	const field = {
		service: '{{json .CreatedAt}}',
		network: '{{json .Created}}',
		volume: '{{json .CreatedAt}}',
	}[kind]
	const result = run([kind, 'inspect', '--format', field, name])
	if (!result.ok) return null
	const at = Date.parse(JSON.parse(result.out) as string)
	return Number.isNaN(at) ? null : at
}

/**
 * Borra lo de corridas anteriores cortadas antes de su limpieza (un SIGKILL): los proyectos
 * it7p<id> cuyos servicios, redes y volúmenes tienen todos más de 30 min. Uno con algo más
 * nuevo puede ser de otra corrida que sigue viva, y no se toca.
 */
export async function sweepStale(log: (line: string) => void): Promise<void> {
	const newest = new Map<string, number>()
	for (const kind of ['service', 'network', 'volume'] as const) {
		for (const name of list(kind)) {
			const project = STALE_PROJECT.exec(name)?.[0]
			if (!project) continue
			const at = createdAt(kind, name) ?? Date.now()
			newest.set(project, Math.max(newest.get(project) ?? 0, at))
		}
	}
	for (const [project, at] of newest) {
		if (Date.now() - at < STALE_AFTER_MS) continue
		log(`it7: borrando ${project}, de una corrida anterior`)
		const left = await removeOwned(project)
		if (left.length > 0) log(`it7: no se pudo borrar: ${left.join(', ')}`)
	}
}
