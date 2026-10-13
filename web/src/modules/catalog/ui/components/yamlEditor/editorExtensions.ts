import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { yaml } from '@codemirror/lang-yaml'
import {
	bracketMatching,
	HighlightStyle,
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
import { tags } from '@lezer/highlight'
import { editorTokens as t } from './styles.css'

// El editor está en un shadow root: su tema va en `EditorView.theme` y el resaltado en un
// `HighlightStyle` con colores, los dos con las variables CSS del tema (que cruzan la
// frontera del shadow root). CodeMirror los monta como hojas construidas, que la CSP
// (`style-src 'self'`) permite.

const ERROR_LINE = 'cm-muelle-error-line'

const darkTheme = EditorView.theme(
	{
		'&': {
			backgroundColor: t.background,
			color: t.text,
			fontSize: t.fontSize,
		},
		'&.cm-focused': { outline: 'none' },
		'.cm-scroller': {
			fontFamily: t.fontFamily,
			lineHeight: '1.75',
			minHeight: '320px',
			maxHeight: '70vh',
			overflow: 'auto',
		},
		'.cm-content': { padding: t.contentPadding, caretColor: t.text },
		'.cm-line': { paddingInline: t.linePadding },
		'.cm-gutters': { backgroundColor: t.background, border: 'none', color: t.gutter },
		'.cm-lineNumbers .cm-gutterElement': {
			minWidth: '32px',
			padding: t.gutterPadding,
			textAlign: 'right',
		},
		'.cm-activeLine': { backgroundColor: t.activeLine },
		'.cm-activeLineGutter': { backgroundColor: 'transparent', color: t.activeGutter },
		'.cm-cursor, .cm-dropCursor': { borderLeftColor: t.text },
		'&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
			{ backgroundColor: t.selection },
		'.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
			backgroundColor: t.matchingBracket,
			outline: `1px solid ${t.matchingBracketOutline}`,
		},
		[`.${ERROR_LINE}`]: { backgroundColor: t.errorLine },
	},
	{ dark: true },
)

const yamlHighlight = HighlightStyle.define([
	{ tag: tags.propertyName, color: t.key },
	{ tag: [tags.content, tags.attributeValue], color: t.value },
	{ tag: [tags.string, tags.special(tags.string)], color: t.string },
	{ tag: tags.lineComment, color: t.comment, fontStyle: 'italic' },
	{
		tag: [tags.separator, tags.punctuation, tags.squareBracket, tags.brace, tags.meta],
		color: t.punctuation,
	},
	{ tag: [tags.labelName, tags.typeName, tags.keyword], color: t.special },
])

/** Marca las líneas con errores de sintaxis (1 = la primera). */
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
