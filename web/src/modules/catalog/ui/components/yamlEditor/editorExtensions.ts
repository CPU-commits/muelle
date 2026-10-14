import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { yaml } from '@codemirror/lang-yaml'
import {
	bracketMatching,
	indentOnInput,
	indentUnit,
	syntaxHighlighting,
} from '@codemirror/language'
import {
	EditorState,
	type Extension,
	RangeSetBuilder,
	StateEffect,
	StateField,
} from '@codemirror/state'
import {
	Decoration,
	type DecorationSet,
	drawSelection,
	EditorView,
	highlightActiveLine,
	highlightActiveLineGutter,
	highlightSpecialChars,
	keymap,
	lineNumbers,
} from '@codemirror/view'
import { darkTheme, ERROR_LINE, yamlHighlight } from './editorTheme'

/** Marca las líneas con errores (1 = la primera). */
export const setErrorLines = StateEffect.define<readonly number[]>()

const errorLines = StateField.define<DecorationSet>({
	create: () => Decoration.none,
	update(decorations, transaction) {
		let next = decorations.map(transaction.changes)
		for (const effect of transaction.effects) {
			if (!effect.is(setErrorLines)) continue
			const builder = new RangeSetBuilder<Decoration>()
			const doc = transaction.state.doc
			for (const line of [...effect.value].sort((a, b) => a - b)) {
				if (line < 1 || line > doc.lines) continue
				const { from } = doc.line(line)
				builder.add(from, from, Decoration.line({ class: ERROR_LINE }))
			}
			next = builder.finish()
		}
		return next
	},
	provide: (field) => EditorView.decorations.from(field),
})

export interface EditorOptions {
	label: string
	/**
	 * El texto de ayuda (cómo salir del editor con el teclado). Va como `aria-description`:
	 * un `aria-describedby` no cruza el shadow root.
	 */
	description?: string
	onChange: (value: string) => void
}

export function editorExtensions(options: EditorOptions): Extension[] {
	return [
		lineNumbers(),
		highlightActiveLineGutter(),
		highlightSpecialChars(),
		history(),
		drawSelection(),
		indentOnInput(),
		bracketMatching(),
		highlightActiveLine(),
		EditorState.tabSize.of(2),
		indentUnit.of('  '),
		// Tab indenta; Esc y después Tab sale del editor (lo explica el hint de la pantalla).
		keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
		yaml(),
		syntaxHighlighting(yamlHighlight),
		errorLines,
		darkTheme,
		EditorView.contentAttributes.of({
			'aria-label': options.label,
			...(options.description ? { 'aria-description': options.description } : {}),
			spellcheck: 'false',
			autocapitalize: 'off',
			autocorrect: 'off',
		}),
		EditorView.updateListener.of((update) => {
			if (update.docChanged) options.onChange(update.state.doc.toString())
		}),
	]
}
