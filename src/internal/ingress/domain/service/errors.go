package service

import "errors"

// ErrNotStageNetwork: ya existe una red de Docker con el nombre de la red del
// stage (proyecto_stage) que no es overlay attachable. No es de muelle: no se
// usa ni se borra. Se responde 409 network_conflict.
var ErrNotStageNetwork = errors.New("ya existe una red de Docker con el nombre del stage que no es overlay attachable; bórrala o renómbrala")
