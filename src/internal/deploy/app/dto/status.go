package dto

import "time"

// AppStatus es GET …/status (contrato §2.8): los servicios de la app en
// Swarm, ordenados por nombre; vacío si nunca se desplegó.
type AppStatus struct {
	Services []ServiceStatus
}

// ServiceStatus es un servicio de la app. Los punteros nil son null.
type ServiceStatus struct {
	Name             string // el nombre corto
	Image            string
	ReplicasDesired  uint64
	ReplicasRunning  uint64
	CPUPercent       *float64 // la suma de las tareas medidas
	MemoryBytes      *uint64
	MemoryLimitBytes *int64 // por tarea
	UpdateState      *string
	Tasks            []TaskStatus
}

// TaskStatus es una tarea de un servicio. Los punteros nil son null.
type TaskStatus struct {
	ID           string
	Slot         *int // nil en un servicio global
	State        string
	DesiredState string
	Health       *string // solo en el nodo local
	Node         *string // el hostname; nil si todavía no tiene nodo
	Since        *time.Time
	Error        *string // enmascarado
	CPUPercent   *float64
	MemoryBytes  *uint64
}
