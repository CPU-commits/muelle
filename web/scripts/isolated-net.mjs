// Corre un comando en un network namespace propio, con solo loopback (Linux).
//
//   node scripts/isolated-net.mjs <comando> [args…]
//
// Por qué: Chromium corta TODAS las peticiones en curso, también las de localhost, con
// ERR_NETWORK_CHANGED cada vez que cambia una dirección IP del host, y Docker cambia una cada
// vez que arranca o borra un contenedor (la veth que cuelga de docker0). En los tests de
// navegador de vitest eso deja archivos con "Failed to fetch dynamically imported module"
// (0 tests) y, si le toca al arranque de un tester, la corrida esperando para siempre.
// Adentro del namespace Chromium solo ve `lo`: lo que pase en la red del host no lo toca. De
// yapa, el puerto del navegador de vitest (63315) es siempre el propio.
//
// Lo usan `pnpm test` (vitest) y `pnpm test:e2e` (Playwright, con el servidor del build que
// levanta adentro). En los e2e, de yapa, los puertos 4173/4174 son del namespace: dos
// worktrees no chocan.
//
// Hace falta `unshare` (util-linux) con user namespaces sin privilegios e `ip` (iproute2). Si
// no están (macOS, un CI que los bloquea), corre el comando con la red del host y lo avisa.
// MUELLE_TEST_HOST_NET=1 lo saltea a propósito, y E2E_REUSE_SERVER=1 también: el servidor
// que se reusa escucha en la red del host, y adentro del namespace no se lo ve.
import { spawn, spawnSync } from 'node:child_process'

const [cmd, ...args] = process.argv.slice(2)
if (!cmd) {
	console.error('uso: node scripts/isolated-net.mjs <comando> [args…]')
	process.exit(2)
}

// `-r` mapea el usuario a root dentro del user namespace: hace falta para levantar `lo`, y
// afuera los archivos que se escriben siguen siendo del usuario.
const UNSHARE = ['--user', '--map-root-user', '--net']
const reuseServer = process.env.E2E_REUSE_SERVER === '1'
const hostNet = process.env.MUELLE_TEST_HOST_NET === '1' || reuseServer

function canIsolate() {
	if (process.platform !== 'linux' || hostNet) return false
	const probe = spawnSync('unshare', [...UNSHARE, 'sh', '-c', 'ip link set lo up'], {
		stdio: 'ignore',
	})
	return probe.status === 0
}

const isolated = canIsolate()
if (reuseServer) {
	console.warn('[isolated-net] E2E_REUSE_SERVER=1: corre con la red del host, la del servidor')
} else if (!isolated && process.platform === 'linux' && !hostNet) {
	console.warn(
		'[isolated-net] no se pudo crear un network namespace (unshare -rn): corre con la red del host',
	)
}

// `exec` en cadena (unshare → sh → comando): el hijo es el comando mismo y le llegan las
// señales que se reenvían abajo.
const [bin, binArgs] = isolated
	? ['unshare', [...UNSHARE, 'sh', '-c', 'ip link set lo up && exec "$@"', 'sh', cmd, ...args]]
	: [cmd, args]
const child = spawn(bin, binArgs, { stdio: 'inherit' })

for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
	process.on(signal, () => child.kill(signal))
}
child.on('error', (error) => {
	console.error(`[isolated-net] ${error.message}`)
	process.exit(1)
})
child.on('exit', (code, signal) => process.exit(code ?? (signal ? 1 : 0)))
