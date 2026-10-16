package compose

// File es el compose final: solo las claves de §6.3 del diseño y las que
// agrega §6.2. Marshal lo serializa; las claves vacías no se escriben.
type File struct {
	Services map[string]Service `json:"services"`
	Volumes  map[string]Volume  `json:"volumes,omitempty"`
	Networks map[string]Network `json:"networks,omitempty"`
}

// Network es una red de primer nivel. En la v1, solo la del stage, externa
// (la crea ingress, §3.2).
type Network struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
}

// Volume es un volumen nombrado del stack: docker stack deploy lo crea como
// <stack>_<nombre> con el driver local (§6.2). Solo lleva etiquetas.
type Volume struct {
	Labels map[string]string `json:"labels,omitempty"`
}

// Service es un servicio del compose final.
type Service struct {
	Image           string            `json:"image"`
	Command         []string          `json:"command,omitempty"`
	Entrypoint      []string          `json:"entrypoint,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	Healthcheck     *Healthcheck      `json:"healthcheck,omitempty"`
	Deploy          *Deploy           `json:"deploy,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Logging         *Logging          `json:"logging,omitempty"`
	Networks        []string          `json:"networks,omitempty"`
	Volumes         []Mount           `json:"volumes,omitempty"`
	Tmpfs           []string          `json:"tmpfs,omitempty"`
	User            string            `json:"user,omitempty"`
	WorkingDir      string            `json:"working_dir,omitempty"`
	StopGracePeriod string            `json:"stop_grace_period,omitempty"`
	StopSignal      string            `json:"stop_signal,omitempty"`
	ReadOnly        bool              `json:"read_only,omitempty"`
	Init            *bool             `json:"init,omitempty"`
	Ulimits         map[string]Ulimit `json:"ulimits,omitempty"`
	CapDrop         []string          `json:"cap_drop,omitempty"`
	Hostname        string            `json:"hostname,omitempty"`
	Domainname      string            `json:"domainname,omitempty"`
	ExtraHosts      []string          `json:"extra_hosts,omitempty"`
	DNS             []string          `json:"dns,omitempty"`
	DNSSearch       []string          `json:"dns_search,omitempty"`
	Expose          []string          `json:"expose,omitempty"`
	Tty             bool              `json:"tty,omitempty"`
	StdinOpen       bool              `json:"stdin_open,omitempty"`
}

// Healthcheck es el healthcheck de un servicio. Las duraciones van como
// time.Duration.String() ("10s").
type Healthcheck struct {
	Test          []string `json:"test,omitempty"`
	Interval      string   `json:"interval,omitempty"`
	Timeout       string   `json:"timeout,omitempty"`
	Retries       *uint64  `json:"retries,omitempty"`
	StartPeriod   string   `json:"start_period,omitempty"`
	StartInterval string   `json:"start_interval,omitempty"`
	Disable       bool     `json:"disable,omitempty"`
}

// Deploy es la sección deploy de un servicio (§6.3).
type Deploy struct {
	Mode           string            `json:"mode,omitempty"`
	Replicas       *uint64           `json:"replicas,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	EndpointMode   string            `json:"endpoint_mode,omitempty"`
	Placement      *Placement        `json:"placement,omitempty"`
	Resources      *Resources        `json:"resources,omitempty"`
	RestartPolicy  *RestartPolicy    `json:"restart_policy,omitempty"`
	UpdateConfig   *UpdateConfig     `json:"update_config,omitempty"`
	RollbackConfig *UpdateConfig     `json:"rollback_config,omitempty"`
}

// Placement dice en qué nodos corren las tareas.
type Placement struct {
	Constraints        []string     `json:"constraints,omitempty"`
	Preferences        []Preference `json:"preferences,omitempty"`
	MaxReplicasPerNode uint64       `json:"max_replicas_per_node,omitempty"`
}

// Preference es una preferencia de ubicación (solo spread).
type Preference struct {
	Spread string `json:"spread"`
}

// Resources son los límites y las reservas de un servicio.
type Resources struct {
	Limits       *Resource `json:"limits,omitempty"`
	Reservations *Resource `json:"reservations,omitempty"`
}

// Resource son las CPU y la memoria de un límite o una reserva. CPUs es un
// número decimal como string ("0.5"); Memory, los bytes como string decimal
// ("536870912"): el esquema de docker stack deploy exige un string en
// memory (verificado con Docker 29.6.1: un entero da "must be a string").
type Resource struct {
	CPUs   string `json:"cpus,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// RestartPolicy es la política de reinicio de las tareas.
type RestartPolicy struct {
	Condition   string  `json:"condition,omitempty"`
	Delay       string  `json:"delay,omitempty"`
	MaxAttempts *uint64 `json:"max_attempts,omitempty"`
	Window      string  `json:"window,omitempty"`
}

// UpdateConfig es update_config o rollback_config.
type UpdateConfig struct {
	Parallelism     *uint64  `json:"parallelism,omitempty"`
	Delay           string   `json:"delay,omitempty"`
	FailureAction   string   `json:"failure_action,omitempty"`
	Monitor         string   `json:"monitor,omitempty"`
	MaxFailureRatio *float64 `json:"max_failure_ratio,omitempty"`
	Order           string   `json:"order,omitempty"`
}

// Logging es el driver de logs y sus opciones.
type Logging struct {
	Driver  string            `json:"driver,omitempty"`
	Options map[string]string `json:"options,omitempty"`
}

// Mount es un montaje de un servicio: un volumen del compose (type volume)
// o un tmpfs (§6.3).
type Mount struct {
	Type     string       `json:"type"`
	Source   string       `json:"source,omitempty"`
	Target   string       `json:"target"`
	ReadOnly bool         `json:"read_only,omitempty"`
	Volume   *MountVolume `json:"volume,omitempty"`
	Tmpfs    *MountTmpfs  `json:"tmpfs,omitempty"`
}

// MountVolume son las opciones de un montaje de volumen.
type MountVolume struct {
	NoCopy bool `json:"nocopy,omitempty"`
}

// MountTmpfs son las opciones de un montaje tmpfs.
type MountTmpfs struct {
	Size int64  `json:"size,omitempty"`
	Mode uint32 `json:"mode,omitempty"`
}

// Ulimit es un ulimit con su valor blando y duro (un entero solo vale para
// los dos).
type Ulimit struct {
	Soft int `json:"soft"`
	Hard int `json:"hard"`
}
