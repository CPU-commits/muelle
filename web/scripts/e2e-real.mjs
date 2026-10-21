// pnpm test:e2e:real [-- <args de playwright>]: el panel contra un muelle de verdad
// (e2e-real/, playwright.real.config.ts).
//
// 1. Exige que el Docker local sea manager de un Swarm activo: un deploy lo necesita. Nunca
//    hace `docker swarm init` (en CI lo hace el job, antes).
// 2. Construye el panel como la imagen (VITE_USE_MOCKS=0 pnpm build, con 200.html, la CSP y
//    sin mockServiceWorker.js), lo copia a src/cmd/http/panel/build/ (ignorado por git) y
//    compila muelle con -tags panel en una carpeta temporal. E2E_REAL_BIN=<binario> se salta
//    los dos pasos y usa ese.
// 3. Baja nginx:alpine si no está (la imagen que despliega el recorrido).
// 4. Toma el candado del Swarm de los tests de integración de Go (flock sobre
//    $TMPDIR/muelle-it-swarm.lock, como dockertest/swarmlock): con otros creando y borrando
//    overlays a la vez, Swarm falla. Y corre Playwright en el network namespace propio de
//    scripts/isolated-net.mjs: Chromium no ve los cambios de red que hace Docker al arrancar
//    contenedores (ERR_NETWORK_CHANGED), y muelle sigue llegando a Docker por su socket.
import { spawn, spawnSync } from 'node:child_process'
import { cpSync, existsSync, mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const root = resolve(web, '..')
const embed = join(root, 'src/cmd/http/panel/build')
const IMAGE = 'nginx:alpine'

function fail(message) {
	console.error(`[e2e-real] ${message}`)
	process.exit(1)
}

function step(message) {
	console.log(`[e2e-real] ${message}`)
}

function run(cmd, args, options = {}) {
	const result = spawnSync(cmd, args, { stdio: 'inherit', ...options })
	if (result.error) fail(`${cmd}: ${result.error.message}`)
	if (result.status !== 0) fail(`${cmd} ${args.join(' ')} terminó con ${result.status}`)
}

function quiet(cmd, args) {
	return spawnSync(cmd, args, { encoding: 'utf8' })
}

const info = quiet('docker', [
	'info',
	'--format',
	'{{.Swarm.LocalNodeState}} {{.Swarm.ControlAvailable}}',
])
if (info.status !== 0 || info.stdout.trim() !== 'active true') {
	fail(
		'el Docker local tiene que ser manager de un Swarm activo (docker swarm init). Este script no lo crea.',
	)
}

let bin = process.env.E2E_REAL_BIN
let binDir = null
if (bin) {
	if (!existsSync(bin)) fail(`E2E_REAL_BIN=${bin} no existe`)
	step(`uso el binario de E2E_REAL_BIN: ${bin}`)
} else {
	step('construyendo el panel (pnpm build, sin MSW)')
	run('pnpm', ['build'], { cwd: web, env: { ...process.env, VITE_USE_MOCKS: '0' } })
	const build = join(web, 'build')
	if (!existsSync(join(build, '200.html')) || !existsSync(join(build, 'csp-header.txt'))) {
		fail('al build le falta 200.html o csp-header.txt')
	}
	if (existsSync(join(build, 'mockServiceWorker.js'))) fail('el build trae mockServiceWorker.js')
	rmSync(embed, { recursive: true, force: true })
	cpSync(build, embed, { recursive: true })

	binDir = mkdtempSync(join(tmpdir(), 'muelle-it7-bin-'))
	bin = join(binDir, 'muelle')
	step('compilando muelle con -tags panel')
	run('go', ['build', '-tags', 'panel', '-o', bin, '.'], {
		cwd: root,
		env: { ...process.env, CGO_ENABLED: '0' },
	})
}

if (quiet('docker', ['image', 'inspect', IMAGE]).status !== 0) {
	step(`bajando ${IMAGE}`)
	run('docker', ['pull', IMAGE])
}

// `pnpm test:e2e:real -- journey` llega con el `--`: lo que sigue es para Playwright.
const extra = process.argv.slice(2)
if (extra[0] === '--') extra.shift()
const playwright = [
	'node',
	'scripts/isolated-net.mjs',
	// El de node_modules: `node scripts/e2e-real.mjs` sin pnpm no lo tiene en el PATH.
	join(web, 'node_modules/.bin/playwright'),
	'test',
	'-c',
	'playwright.real.config.ts',
	...extra,
]
const lock = join(tmpdir(), 'muelle-it-swarm.lock')
let command = playwright
if (quiet('flock', ['--version']).status === 0) {
	step(`esperando el candado del Swarm (${lock})`)
	// --no-fork: flock se reemplaza por el comando (que sigue con el candado), y las señales
	// que se reenvían abajo le llegan a él y no solo a flock.
	command = ['flock', '--no-fork', lock, ...playwright]
} else {
	console.warn('[e2e-real] sin flock: corre sin el candado del Swarm de los tests de Go')
}

const [cmd, ...args] = command
const child = spawn(cmd, args, {
	cwd: web,
	stdio: 'inherit',
	env: { ...process.env, MUELLE_E2E_BIN: bin },
})
for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
	process.on(signal, () => child.kill(signal))
}
child.on('error', (error) => fail(error.message))
child.on('exit', (code, signal) => {
	if (binDir) rmSync(binDir, { recursive: true, force: true })
	process.exit(code ?? (signal ? 1 : 0))
})
