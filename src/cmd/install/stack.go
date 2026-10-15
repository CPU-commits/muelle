package install

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// stackInput es lo que define el stack de muelle (contrato §5.3).
type stackInput struct {
	Names    naming.InstallNames
	Image    string // la imagen de muelle que se despliega
	Origin   string // MUELLE_PANEL_ORIGIN
	PublicIP string // la de --public-ip o la detectada; vacía no va
	NodeID   string // el nodo donde corrió install (node.id)
	Token    bool   // monta el secreto del token de Cloudflare
	Registry bool   // monta el secreto del registry inicial
	// Opts aporta Email, PublicIPv6, HTTPPort, HTTPSPort y CaddyImage.
	Opts Options
}

// Las claves cortas del compose. Con name: dan los nombres de
// naming.InstallNames, y el prefijo de los tests no cambia el documento.
const (
	keyService     = "muelle"
	keyData        = "data"
	keyCaddyAdmin  = "caddy_admin"
	keyMasterKey   = "master_key"
	keyToken       = "init_cloudflare_token"
	keyRegistry    = "init_registry"
	keyPanel       = "panel"
	secretsDir     = "/run/secrets/"
	dockerSocket   = "/var/run/docker.sock"
	muelleDataDir  = "/data"
	muelleListen   = ":8080"
	logMaxSize     = "10m"
	logMaxFile     = "3"
	updateOrder    = "stop-first"
	updateFailure  = "rollback"
	updateMonitor  = "30s"
	managerRole    = "node.role == manager"
	nodeConstraint = "node.id == "
)

// El compose como structs: encoding/json lo escribe en el orden de los
// campos, y los mapas ordenados por clave, así el documento es determinista.
type composeFile struct {
	Services map[string]composeService  `json:"services"`
	Volumes  map[string]composeNamed    `json:"volumes"`
	Secrets  map[string]composeExternal `json:"secrets"`
	Networks map[string]composeExternal `json:"networks"`
}

type composeService struct {
	Image       string            `json:"image"`
	Environment map[string]string `json:"environment"`
	Volumes     []composeMount    `json:"volumes"`
	Secrets     []composeSecret   `json:"secrets"`
	Networks    []string          `json:"networks"`
	Logging     composeLogging    `json:"logging"`
	Deploy      composeDeploy     `json:"deploy"`
}

type composeMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type composeSecret struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type composeLogging struct {
	Driver  string            `json:"driver"`
	Options map[string]string `json:"options"`
}

type composeDeploy struct {
	Replicas       int              `json:"replicas"`
	Placement      composePlacement `json:"placement"`
	UpdateConfig   composeUpdate    `json:"update_config"`
	RollbackConfig composeUpdate    `json:"rollback_config"`
}

type composePlacement struct {
	Constraints []string `json:"constraints"`
}

type composeUpdate struct {
	Order         string `json:"order"`
	FailureAction string `json:"failure_action,omitempty"`
	Monitor       string `json:"monitor,omitempty"`
}

type composeNamed struct {
	Name string `json:"name"`
}

type composeExternal struct {
	External bool   `json:"external"`
	Name     string `json:"name"`
}

// serviceEnv son las variables del servicio de muelle (contrato §5.2): las
// fijas, MUELLE_INIT_PROXY_* siempre, y las demás solo con valor.
// MUELLE_SECURE_COOKIES no va (true).
func serviceEnv(in stackInput) map[string]string {
	env := map[string]string{
		"MUELLE_PANEL_ORIGIN":          in.Origin,
		"MUELLE_LISTEN":                muelleListen,
		"MUELLE_DATA_DIR":              muelleDataDir,
		"MUELLE_MASTER_KEY_FILE":       secretsDir + naming.MasterKeySecret,
		"MUELLE_INIT_ACME_EMAIL":       in.Opts.Email,
		"MUELLE_INIT_PROXY_HTTP_PORT":  strconv.Itoa(in.Opts.HTTPPort),
		"MUELLE_INIT_PROXY_HTTPS_PORT": strconv.Itoa(in.Opts.HTTPSPort),
	}
	optional := map[string]string{
		"MUELLE_INIT_PUBLIC_IP":   in.PublicIP,
		"MUELLE_INIT_PUBLIC_IPV6": in.Opts.PublicIPv6,
		"MUELLE_INIT_CADDY_IMAGE": in.Opts.CaddyImage,
	}
	for k, v := range optional {
		if v != "" {
			env[k] = v
		}
	}
	if in.Token {
		env["MUELLE_INIT_CLOUDFLARE_TOKEN_FILE"] = secretsDir + naming.InitCloudflareTokenSecret
	}
	if in.Registry {
		env["MUELLE_INIT_REGISTRY_FILE"] = secretsDir + naming.InitRegistrySecret
	}
	return env
}

// composeStack arma el compose del stack de muelle (contrato §5.3) como JSON,
// sin version: (el CLI 29 lo lee como 3.13). docker stack deploy interpola
// ${VAR} con su propio entorno, así que todo "$" del documento sale como
// "$$": el reemplazo se hace sobre el JSON ya escrito, que solo tiene "$"
// dentro de los strings (encoding/json no lo escapa), y así ningún string
// queda sin escapar.
func composeStack(in stackInput) ([]byte, error) {
	n := in.Names
	svc := composeService{
		Image:       in.Image,
		Environment: serviceEnv(in),
		Volumes: []composeMount{
			{Type: "bind", Source: dockerSocket, Target: dockerSocket},
			{Type: "volume", Source: keyData, Target: muelleDataDir},
			{Type: "volume", Source: keyCaddyAdmin, Target: naming.CaddyAdminDir},
		},
		// El target es siempre el nombre fijo: MUELLE_*_FILE no cambia con el prefijo.
		Secrets:  []composeSecret{{Source: keyMasterKey, Target: naming.MasterKeySecret}},
		Networks: []string{keyPanel},
		Logging:  composeLogging{Driver: "json-file", Options: map[string]string{"max-size": logMaxSize, "max-file": logMaxFile}},
		Deploy: composeDeploy{
			Replicas:       1,
			Placement:      composePlacement{Constraints: []string{managerRole, nodeConstraint + in.NodeID}},
			UpdateConfig:   composeUpdate{Order: updateOrder, FailureAction: updateFailure, Monitor: updateMonitor},
			RollbackConfig: composeUpdate{Order: updateOrder},
		},
	}
	secrets := map[string]composeExternal{keyMasterKey: {External: true, Name: n.MasterKeySecret}}
	if in.Token {
		svc.Secrets = append(svc.Secrets, composeSecret{Source: keyToken, Target: naming.InitCloudflareTokenSecret})
		secrets[keyToken] = composeExternal{External: true, Name: n.InitCloudflareTokenSecret}
	}
	if in.Registry {
		svc.Secrets = append(svc.Secrets, composeSecret{Source: keyRegistry, Target: naming.InitRegistrySecret})
		secrets[keyRegistry] = composeExternal{External: true, Name: n.InitRegistrySecret}
	}
	doc := composeFile{
		Services: map[string]composeService{keyService: svc},
		Volumes:  map[string]composeNamed{keyData: {Name: n.DataVolume}, keyCaddyAdmin: {Name: n.CaddyAdminVolume}},
		Secrets:  secrets,
		Networks: map[string]composeExternal{keyPanel: {External: true, Name: n.PanelNetwork}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // "<" y "&" tal cual: es para el CLI, no para un HTML
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(buf.Bytes(), []byte("$"), []byte("$$")), nil
}
