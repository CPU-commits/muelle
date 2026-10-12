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
import {
	errorLine,
	tokenComment,
	tokenKey,
	tokenPunctuation,
	tokenSpecial,
	tokenString,
	tokenValue,
} from './styles.css'

// Los colores van por clase (los tokens del tema), no con estilos en línea.
const yamlHighlight = HighlightStyle.define([
	{ tag: tags.propertyName, class: tokenKey },
	{ tag: [tags.content, tags.attributeValue], class: tokenValue },
	{ tag: [tags.string, tags.special(tags.string)], class: tokenString },
	{ tag: tags.lineComment, class: tokenComment },
	{
		tag: [tags.separator, tags.punctuation, tags.squareBracket, tags.brace, tags.meta],
		class: tokenPunctuation,
	},
	{ tag: [tags.labelName, tags.typeName, tags.keyword], class: tokenSpecial },
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
				builder.add(from, from, Decoration.line({ class: errorLine }))
			}
			next = builder.finish()
		}
		return next
	},
	provide: (field) => EditorView.decorations.from(field),
})

export interface EditorOptions {
	label: string
	describedBy?: string
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
		EditorView.darkTheme.of(true),
		EditorView.contentAttributes.of({
			'aria-label': options.label,
			...(options.describedBy ? { 'aria-describedby': options.describedBy } : {}),
			spellcheck: 'false',
			autocapitalize: 'off',
			autocorrect: 'off',
		}),
		EditorView.updateListener.of((update) => {
			if (update.docChanged) options.onChange(update.state.doc.toString())
		}),
	]
}
