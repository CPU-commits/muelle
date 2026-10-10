import { createGlobalTheme } from '@vanilla-extract/css'
import { darkTheme } from './dark.theme'

// Las variables se declaran en :root y no en una clase, para que `body` y los estilos
// globales puedan leerlas. muelle tiene un solo tema (oscuro); para sumar otro habría
// que pasar a `createGlobalThemeContract` + un `createGlobalTheme` por selector.
export const theme = createGlobalTheme(':root', darkTheme)
