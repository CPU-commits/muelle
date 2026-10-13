export { lineCount } from './lines'

// CodeMirror (el editor, la gramática de YAML y el tema) pesa unos 380 kB: no puede ir en
// el bundle de Proyectos ni del layout de una app. Se carga recién cuando se abre el
// editor de un compose; la validación de sintaxis usa la misma gramática y viaja con él.
export const loadYamlEditor = () => import('./YamlEditor.svelte')
export const loadYamlSyntax = () => import('./yamlSyntax')
