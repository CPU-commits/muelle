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
