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

// LogPage es un pedazo del log (contrato §2.5). Length es el largo del log
// al leerlo (read_deploy_log por MCP, contrato §8.3 #10; REST no lo manda).
type LogPage struct {
	Chunk        string
	NextOffset   int
	Length       int
	Status       entity.Status
	SupersededBy *int64
}

// LogLine es una línea del log de un deploy para el stream (contrato §2.6):
// el texto sin el \n, y el offset después de ella (el id del evento).
type LogLine struct {
	Text   string
	Offset int
}

// TailLog pide el final del log de un deploy: a lo más Bytes del log
// (get_deploy por MCP, contrato §8.3 #9).
type TailLog struct {
	DeployRef
	Bytes int
}

// LogTail es el final del log en líneas completas: Lines con el offset
// después de cada una, desde FromOffset; Length es el largo del log al
// leerlo.
type LogTail struct {
	Lines        []LogLine
	FromOffset   int
	Length       int
	Status       entity.Status
	SupersededBy *int64
}
