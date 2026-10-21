// Package dto tiene las entradas de los casos de uso de secrets.
package dto

// EnvTarget identifica por slugs al dueño de las variables. Con App vacío, el dueño es el stage.
type EnvTarget struct {
	Project string
	Stage   string
	App     string
}

// ReplaceEnv reemplaza todo el set de variables (PUT). Vars es obligatorio.
type ReplaceEnv struct {
	Target EnvTarget
	Vars   map[string]string
	Secret *bool // aplica a todas; nil es true
}

// PatchEnv agrega o reemplaza las variables de Set y borra las claves de Unset (PATCH).
type PatchEnv struct {
	Target EnvTarget
	Set    map[string]string
	Unset  []string
	Secret *bool // aplica a las de Set; nil es true
}

// ResolveEnv pide las variables que recibe una app al desplegarse (Fase 2).
// El stage sale de la propia app.
type ResolveEnv struct {
	AppID int64
}

// ResolvedEnv es lo que recibe el deploy. Nada de esto va a la API ni a la auditoría.
type ResolvedEnv struct {
	// Vars son las del stage con las de la app encima (la app gana).
	Vars map[string]string
	// SecretValues son los valores de las variables secretas del stage y de
	// la app (también los que la app pisa), sin repetidos ni vacíos y los más
	// largos primero: el deploy los reemplaza por *** en su log (§7).
	SecretValues []string
}
