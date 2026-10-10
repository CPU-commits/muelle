// Package entity tiene las entidades de secrets: las variables de entorno y su dueño.
package entity

// OwnerKind es el tipo de dueño de un set de variables.
type OwnerKind string

const (
	OwnerStage OwnerKind = "stage"
	OwnerApp   OwnerKind = "app"
)

// Owner identifica al dueño de un set de variables: un stage o una app.
// Los ids vienen de catalog (tablas stages y apps).
type Owner struct {
	Kind OwnerKind
	ID   int64
}

func StageOwner(id int64) Owner { return Owner{Kind: OwnerStage, ID: id} }
func AppOwner(id int64) Owner   { return Owner{Kind: OwnerApp, ID: id} }

// Valid dice si el dueño es un stage o una app con un id válido. El Owner
// vacío no es válido: así un dueño sin resolver nunca llega a la base ni al cifrado.
func (o Owner) Valid() bool {
	return (o.Kind == OwnerStage || o.Kind == OwnerApp) && o.ID > 0
}
