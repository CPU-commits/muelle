// Los proyectos y stages entre los que se elige el alcance de un token. Son de `catalog`,
// pero este módulo solo necesita su slug (y el nombre para mostrar): los lee con su propio
// datasource para no depender de las tripas de otro módulo.

export interface ProjectRef {
	readonly slug: string
	readonly name: string
}

export interface StageRef {
	readonly slug: string
}
