import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El host del shadow root (ShadowHost): el fondo y el borde de foco se ven desde la
// página. Lo de adentro (CodeMirror) se estila con su propio tema: editorTheme.ts.
export const root = style({
	minWidth: 0,
	backgroundColor: theme.colors.background.page,
	transition: `box-shadow ${theme.transition.fast}`,
	selectors: {
		'&:focus-within': { boxShadow: `inset 0 0 0 1px ${theme.colors.borders.focus}` },
	},
})
