import adapter from '@sveltejs/adapter-static'
import { createHash } from 'node:crypto'
import { readFile, rm, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { loadEnv } from 'vite'

// `pnpm build` escribe en ./build, lo que embebe el binario de Go. Los e2e construyen en
// otra carpeta (BUILD_DIR, ver playwright.config.ts) para no pisarlo con un build con MSW.
const outDir = process.env.BUILD_DIR || 'build'
// El mismo valor que ve el bundle (la variable del proceso, o si no un `.env`).
const withMocks = loadEnv('production', process.cwd(), 'VITE_').VITE_USE_MOCKS === '1'

// SvelteKit monta un <div id="svelte-announcer"> (anuncia cada navegación a los lectores
// de pantalla) con este `style` en línea (`core/sync/write_root.js` de @sveltejs/kit).
// Bajo `style-src 'self'` el navegador lo bloquea y el div queda visible al pie de la
// página. `'unsafe-hashes'` + su hash deja pasar ESE atributo y ningún otro. Si una
// versión nueva de SvelteKit lo cambia, lo avisa e2e/csp.e2e.ts.
const ANNOUNCER_STYLE =
	'position: absolute; left: 0; top: 0; clip: rect(0 0 0 0); clip-path: inset(50%); overflow: hidden; white-space: nowrap; width: 1px; height: 1px'
const announcerHash = `sha256-${createHash('sha256').update(ANNOUNCER_STYLE).digest('base64')}`

// La política del panel. Va como <meta> en el `200.html`, con el hash del script de
// arranque que agrega SvelteKit; el header que manda Go es esta misma política más lo
// que un <meta> no puede llevar (ver `csp-header.txt` abajo y web/CLAUDE.md).
/** @type {import('@sveltejs/kit').CspDirectives} */
const directives = {
	'default-src': ['self'],
	'script-src': ['self'],
	'style-src': ['self'],
	'style-src-attr': ['unsafe-hashes', /** @type {`sha256-${string}`} */ (announcerHash)],
	'img-src': ['self'],
	'font-src': ['self'],
	'connect-src': ['self'],
	'worker-src': ['self'],
	'object-src': ['none'],
	'base-uri': ['none'],
	'form-action': ['self'],
}

// Lo que el navegador ignora en un <meta>: solo sirve como header.
const HEADER_ONLY = ["frame-ancestors 'none'"]

const META_CSP = /<meta http-equiv="content-security-policy" content="([^"]+)"/

/**
 * adapter-static + dos pasos al final:
 * - fuera de un build con mocks, borra `mockServiceWorker.js` (lo copia `static/`);
 * - escribe `csp-header.txt`: el valor exacto del header Content-Security-Policy que tiene
 *   que mandar el servidor (el <meta> del `200.html` + `frame-ancestors 'none'`). El hash
 *   del script de arranque cambia en cada build, así que no se puede escribir a mano.
 * @param {import('@sveltejs/adapter-static').AdapterOptions} options
 * @returns {import('@sveltejs/kit').Adapter}
 */
function panelAdapter(options) {
	const base = adapter(options)
	return {
		...base,
		async adapt(builder) {
			await base.adapt(builder)
			if (!withMocks) await rm(join(outDir, 'mockServiceWorker.js'), { force: true })

			const shell = await readFile(join(outDir, '200.html'), 'utf8')
			const meta = META_CSP.exec(shell)?.[1]
			if (!meta) throw new Error('El 200.html no trae la CSP en un <meta>: revisá kit.csp')
			const policy = [meta.replaceAll('&#39;', "'").replace(/;\s*$/, ''), ...HEADER_ONLY]
			await writeFile(join(outDir, 'csp-header.txt'), `${policy.join('; ')}\n`)
		},
	}
}

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// SPA estricta: un solo shell (`200.html`) que el binario de Go devuelve para
		// cualquier ruta que no sea un archivo del build ni `/api`, y el router resuelve
		// del lado del cliente. No hay proceso Node en producción, así que la capa `data`
		// corre en un solo lado: el navegador.
		adapter: panelAdapter({
			pages: outDir,
			assets: outDir,
			fallback: '200.html',
			precompress: false,
			strict: false,
		}),
		// Hash, no nonce: el `200.html` es estático (lo genera el build), así que no hay
		// un servidor que invente un nonce por respuesta.
		csp: { mode: 'hash', directives },
		alias: {
			'@': './src',
		},
	},
	vitePlugin: {
		dynamicCompileOptions: ({ filename }) =>
			filename.includes('node_modules') ? undefined : { runes: true },
	},
}

export default config
