// Sirve uno o más builds del panel como los sirve el binario de Go con `-tags panel`
// (contrato §5.5, `src/cmd/http/panel/panel.go`). Solo para los e2e:
// - el archivo del build si existe con ese path (un directorio cuenta como que no existe);
//   si no, el `200.html`, que resuelve el router del panel. `/` es el `200.html`;
// - un `/_app/…` que no existe es 404 en texto (un chunk viejo nunca recibe HTML), y
//   `csp-header.txt` no se sirve (responde el `200.html`);
// - solo GET y HEAD: otro método es 405 en JSON, con `Allow: GET, HEAD`;
// - en TODAS las respuestas, `Content-Security-Policy` (el contenido de `csp-header.txt`),
//   `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin` y `Cache-Control`: un
//   año e `immutable` en `/_app/immutable/…`, `no-cache` en todo lo demás;
// - el `Content-Type` sale de la tabla de panel.go, por la extensión en minúscula.
//
// Lo que NO copia (los e2e no lo necesitan, y por eso no lo prueban): el `ETag` de lo que
// está fuera de `/_app/immutable/` y su `304` con `If-None-Match`, y los `Range` que atiende
// `http.ServeContent`. Que este servidor y panel.go digan lo mismo no lo verifica nada: si
// cambia panel.go, se cambia acá a mano.
//
//   node scripts/serve-build.mjs <carpeta>:<puerto> [<carpeta>:<puerto> …]
//
// No hay backend: `/api/*` contesta 401 `unauthenticated`, lo mismo que la API real sin
// cookie. Un build con MSW no llega acá (el service worker contesta antes).
import { createServer } from 'node:http'
import { readFile, stat } from 'node:fs/promises'
import { extname, join, posix, resolve, sep } from 'node:path'

const SHELL = '200.html'
const CSP_FILE = 'csp-header.txt'
const IMMUTABLE = '_app/immutable/'

// La tabla de tipos de panel.go (sin depender de los del sistema).
const TYPES = {
	'.html': 'text/html; charset=utf-8',
	'.js': 'text/javascript; charset=utf-8',
	'.mjs': 'text/javascript; charset=utf-8',
	'.css': 'text/css; charset=utf-8',
	'.json': 'application/json',
	'.map': 'application/json',
	'.webmanifest': 'application/manifest+json',
	'.txt': 'text/plain; charset=utf-8',
	'.xml': 'application/xml',
	'.svg': 'image/svg+xml',
	'.png': 'image/png',
	'.jpg': 'image/jpeg',
	'.jpeg': 'image/jpeg',
	'.gif': 'image/gif',
	'.webp': 'image/webp',
	'.avif': 'image/avif',
	'.ico': 'image/x-icon',
	'.woff': 'font/woff',
	'.woff2': 'font/woff2',
	'.ttf': 'font/ttf',
	'.otf': 'font/otf',
	'.wasm': 'application/wasm',
}

/** `path.Clean("/" + path)` sin la `/` del principio: `""` es la raíz. */
function cleanName(pathname) {
	let decoded
	try {
		decoded = decodeURIComponent(pathname)
	} catch {
		return null
	}
	return posix.normalize(`/${decoded}`).replace(/^\/+/, '').replace(/\/+$/, '')
}

/** El archivo del build que corresponde a `name`, si es uno que se sirve. */
async function fileFor(root, name) {
	if (name === '' || name === CSP_FILE) return null
	const file = resolve(root, name)
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
	const csp = (await readFile(join(root, CSP_FILE), 'utf8')).trim()
	const shell = await readFile(join(root, SHELL))

	const server = createServer(async (request, response) => {
		const { pathname } = new URL(request.url ?? '/', 'http://panel')

		if (pathname === '/api' || pathname.startsWith('/api/')) {
			response.writeHead(401, { 'Content-Type': 'application/json' })
			response.end(
				JSON.stringify({ error: { code: 'unauthenticated', message: 'sin backend' } }),
			)
			return
		}

		response.setHeader('Content-Security-Policy', csp)
		response.setHeader('X-Content-Type-Options', 'nosniff')
		response.setHeader('Referrer-Policy', 'same-origin')
		response.setHeader('Cache-Control', 'no-cache')

		if (request.method !== 'GET' && request.method !== 'HEAD') {
			response.writeHead(405, { 'Content-Type': 'application/json', Allow: 'GET, HEAD' })
			response.end(
				JSON.stringify({
					error: { code: 'method_not_allowed', message: 'método no permitido' },
				}),
			)
			return
		}
		const head = request.method === 'HEAD'

		const name = cleanName(pathname) ?? ''
		const file = await fileFor(root, name)
		if (file) {
			const type = TYPES[extname(file).toLowerCase()] ?? 'application/octet-stream'
			const headers = { 'Content-Type': type }
			if (name.startsWith(IMMUTABLE)) {
				headers['Cache-Control'] = 'public, max-age=31536000, immutable'
			}
			response.writeHead(200, headers)
			response.end(head ? undefined : await readFile(file))
			return
		}
		if (name === '_app' || name.startsWith('_app/')) {
			response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
			response.end(head ? undefined : 'no existe\n')
			return
		}
		response.writeHead(200, { 'Content-Type': TYPES['.html'] })
		response.end(head ? undefined : shell)
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
