import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// --- "Agregar variable" (App-Variables.dc.html) -------------------------------------

export const addCard = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
	padding: `${theme.space[16]} ${theme.space[20]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
})

export const addTitle = style({
	fontSize: theme.typography.fontSize[14],
	fontWeight: theme.typography.fontWeight.semibold,
})

// Clave (1), valor (2), secreta y el botón: en un teléfono cada uno baja a su línea.
export const addForm = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: theme.space[8],
})

export const keyField = style({
	flex: '1 1 220px',
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
})

export const valueField = style([keyField, { flex: '2 1 280px' }])

export const fieldError = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.errorText,
})

export const secretToggle = style({
	flex: 'none',
	height: theme.control.md,
	paddingInline: theme.space[4],
})

export const submitSlot = style({
	flex: 'none',
})

export const note = style({
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

// --- Lista ------------------------------------------------------------------------

export const listSection = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
})

export const listHeader = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
})

export const listHeading = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: `${theme.space[4]} ${theme.space[10]}`,
	minWidth: 0,
})

export const listTitle = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const listMeta = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const searchBox = style({
	flex: '0 1 280px',
	minWidth: 0,
})

export const noResults = style({
	padding: `${theme.space[16]} ${theme.space[20]}`,
	border: `1px dashed ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

// Las columnas de una fila (clave, valor, cuándo, ⋯) envuelven en un teléfono.
export const keyCell = style({
	flex: '1 1 220px',
	minWidth: 0,
	overflowWrap: 'anywhere',
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
})

export const valueCell = style({
	flex: '2 1 280px',
	minWidth: 0,
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[8],
})

export const updatedCell = style({
	flex: '0 0 130px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

// --- Editar -----------------------------------------------------------------------

export const editForm = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
})

export const editNote = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.warnText,
})
