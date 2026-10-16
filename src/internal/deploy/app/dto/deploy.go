package dto

import "github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"

// CreateDeploy es POST …/deploys (contrato §2.2): Image en una app de
// formulario, o Images por servicio; ninguno de los dos redespliega.
type CreateDeploy struct {
	AppRef
	Image  *string
	Images map[string]string // nil si el cuerpo no trae images
}

// Queued es la respuesta de un deploy o un rollback encolado (202).
type Queued struct {
	ID        int64
	Status    entity.Status
	StatusURL string
}

// DeployRef identifica un deploy de una app.
type DeployRef struct {
	AppRef
	ID int64
}

// ReadLog es GET …/deploys/{id}/log?offset=N (contrato §2.5).
type ReadLog struct {
	DeployRef
	Offset int
}

// LogPage es un pedazo del log (contrato §2.5).
type LogPage struct {
	Chunk        string
	NextOffset   int
	Status       entity.Status
	SupersededBy *int64
}

// LogLine es una línea del log de un deploy para el stream (contrato §2.6):
// el texto sin el \n, y el offset después de ella (el id del evento).
type LogLine struct {
	Text   string
	Offset int
}
