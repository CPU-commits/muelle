// Sirve uno o más builds del panel como los va a servir el binario de Go (F2): el
// archivo del build si existe, `200.html` para cualquier otra ruta, y en TODAS las
// respuestas el header Content-Security-Policy de `csp-header.txt`. Solo para los e2e.
//
//   node scripts/serve-build.mjs <carpeta>:<puerto> [<carpeta>:<puerto> …]
//
// No hay backend: `/api/*` contesta 401 `unauthenticated`, lo mismo que la API real sin
// cookie. Un build con MSW no llega acá (el service worker contesta antes).
import { createServer } from 'node:http'
import { readFile, stat } from 'node:fs/promises'
import { extname, join, resolve, sep } from 'node:path'

const TYPES = {
	'.html': 'text/html; charset=utf-8',
	'.js': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8',
	'.json': 'application/json',
	'.svg': 'image/svg+xml',
	'.png': 'image/png',
	'.woff2': 'font/woff2',
	'.txt': 'text/plain; charset=utf-8',
}

/** @param {string} root @param {string} pathname */
async function fileFor(root, pathname) {
	const file = resolve(root, `.${decodeURIComponent(pathname)}`)
	if (!file.startsWith(root + sep)) return null
	try {
		return (await stat(file)).isFile() ? file : null
	} catch {
		return null
	}
}

/** @param {string} dir @param {number} port */
async function serve(dir, port) {
	const root = resolve(dir)
	const csp = (await readFile(join(root, 'csp-header.txt'), 'utf8')).trim()
	const shell = await readFile(join(root, '200.html'))

	const server = createServer(async (request, response) => {
		const { pathname } = new URL(request.url ?? '/', 'http://panel')
		response.setHeader('Content-Security-Policy', csp)
		response.setHeader('X-Content-Type-Options', 'nosniff')

		if (pathname.startsWith('/api/')) {
			response.writeHead(401, { 'Content-Type': 'application/json' })
			response.end(
				JSON.stringify({ error: { code: 'unauthenticated', message: 'sin backend' } }),
			)
			return
		}

		const file = await fileFor(root, pathname)
		if (file) {
			response.writeHead(200, {
				'Content-Type': TYPES[extname(file)] ?? 'application/octet-stream',
			})
			response.end(await readFile(file))
			return
		}
		if (pathname.startsWith('/_app/')) {
			response.writeHead(404).end()
			return
		}
		response.writeHead(200, { 'Content-Type': TYPES['.html'], 'Cache-Control': 'no-cache' })
		response.end(shell)
	})

	await new Promise((done) => server.listen(port, '127.0.0.1', () => done(undefined)))
	console.log(`${dir} en http://127.0.0.1:${port}`)
}

const targets = process.argv.slice(2).map((arg) => {
	const at = arg.lastIndexOf(':')
	return { dir: arg.slice(0, at), port: Number(arg.slice(at + 1)) }
})
if (targets.length === 0 || targets.some(({ dir, port }) => !dir || !Number.isInteger(port))) {
	console.error('uso: node scripts/serve-build.mjs <carpeta>:<puerto> [<carpeta>:<puerto> …]')
	process.exit(2)
}
for (const { dir, port } of targets) await serve(dir, port)
