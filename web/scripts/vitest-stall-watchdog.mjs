// Reporter de vitest que corta una corrida colgada en vez de dejarla esperando para siempre.
//
// En modo navegador, vitest 4 espera sin timeout a que cada tester (el iframe que corre un
// archivo) arranque y conteste: si el arranque del tester se cae (un import de su runtime
// que el navegador cortó), la corrida queda colgada al 0 % de CPU y testTimeout/hookTimeout
// no la salvan (no hay test corriendo). Este reporter mira que siga habiendo progreso
// (archivos, suites, tests, hooks); si pasan `stallMs` sin nada, lista los archivos que no
// terminaron y sale con código 1.
//
// En vite.config.ts: `reporters: ['default', ['./scripts/vitest-stall-watchdog.mjs', { stallMs }]]`.
import { relative } from 'node:path'

const DEFAULT_STALL_MS = 120_000
const CHECK_EVERY_MS = 5_000

export default class StallWatchdog {
	/** @param {{ stallMs?: number }} [options] */
	constructor(options = {}) {
		this.stallMs = options.stallMs ?? DEFAULT_STALL_MS
		/** @type {any} */
		this.vitest = undefined
		/** @type {{ key: string, label: string }[]} */
		this.specs = []
		/** @type {Set<string>} */
		this.done = new Set()
		this.lastProgress = Date.now()
		/** @type {ReturnType<typeof setInterval> | undefined} */
		this.timer = undefined
	}

	/** @param {any} vitest */
	onInit(vitest) {
		this.vitest = vitest
	}

	/** @param {readonly any[]} specifications */
	onTestRunStart(specifications) {
		const root = this.vitest?.config.root ?? process.cwd()
		this.specs = specifications.map((spec) => ({
			key: `${spec.project.name}\0${spec.moduleId}`,
			label: `${spec.project.name}: ${relative(root, spec.moduleId)}`,
		}))
		this.done.clear()
		this.progress()
		clearInterval(this.timer)
		this.timer = setInterval(() => this.check(), CHECK_EVERY_MS)
		this.timer.unref()
	}

	/** @param {any} testModule */
	onTestModuleEnd(testModule) {
		this.done.add(`${testModule.project.name}\0${testModule.moduleId}`)
		this.progress()
	}

	// Cualquier otro evento de la corrida también cuenta como progreso.
	onTestModuleQueued() {
		this.progress()
	}
	onTestModuleCollected() {
		this.progress()
	}
	onTestModuleStart() {
		this.progress()
	}
	onTestSuiteReady() {
		this.progress()
	}
	onTestSuiteResult() {
		this.progress()
	}
	onTestCaseReady() {
		this.progress()
	}
	onTestCaseResult() {
		this.progress()
	}
	onHookStart() {
		this.progress()
	}
	onHookEnd() {
		this.progress()
	}

	onTestRunEnd() {
		clearInterval(this.timer)
		this.timer = undefined
	}

	progress() {
		this.lastProgress = Date.now()
	}

	check() {
		const idle = Date.now() - this.lastProgress
		if (idle < this.stallMs) return
		clearInterval(this.timer)
		const pending = this.specs.filter((spec) => !this.done.has(spec.key))
		const lines = [
			'',
			`[stall-watchdog] ${Math.round(idle / 1000)} s sin progreso: la corrida está colgada.`,
			`${pending.length} archivo(s) sin terminar:`,
			...pending.slice(0, 30).map((spec) => `  - ${spec.label}`),
			'En modo navegador suele ser un tester que no arrancó (ver web/CLAUDE.md, "Cosas que conviene saber").',
			'',
		]
		;(this.vitest?.logger ?? console).error(lines.join('\n'))
		process.exitCode = 1
		const close = this.vitest?.close?.() ?? Promise.resolve()
		const deadline = new Promise((resolve) => setTimeout(resolve, 10_000).unref())
		Promise.race([close, deadline]).finally(() => process.exit(1))
	}
}
