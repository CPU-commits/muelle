package install

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/distribution/reference"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// Las imágenes de muelle son privadas (plan de la Fase 5, "Cambio de
// requisito del dueño"): install las baja con la credencial de --registry-user
// y --registry-password-stdin o, si no vino, con la del config.json de Docker
// del host, montado en el contenedor.

// privateHint es lo que hay que hacer si no se pudo bajar una imagen.
const privateHint = "las imágenes de muelle son privadas: haz docker login ghcr.io con un token de GitHub con permiso read:packages y monta ~/.docker/config.json " +
	"(docker run … -v ~/.docker/config.json:/root/.docker/config.json:ro), o usa --registry-user y --registry-password-stdin"

// registryHost es el host de una imagen, como lo da reference.Domain
// ("ghcr.io", "docker.io" para Docker Hub): el de la tabla registries
// (contrato §2.10).
func registryHost(ref string) (string, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", fmt.Errorf("%q no es una referencia de imagen", ref)
	}
	return reference.Domain(named), nil
}

// defaultDockerConfig es el config.json que lee el CLI de docker:
// $DOCKER_CONFIG/config.json o ~/.docker/config.json (en la imagen,
// /root/.docker/config.json).
func defaultDockerConfig() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker", "config.json")
}

// resolveCredentials arma la credencial de cada host de refs (la imagen de
// muelle primero; las vacías no cuentan). Con --registry-user es la del host
// de la imagen de muelle, y el config.json no se lee. Si no, sale del
// config.json en configPath; si no existe, no hay credenciales (un pull
// anónimo). Ningún error cita la contraseña.
func resolveCredentials(o Options, configPath string, refs ...string) (map[string]*docker.RegistryAuth, error) {
	var hosts []string
	for _, ref := range refs {
		if ref == "" {
			continue
		}
		h, err := registryHost(ref)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	creds := map[string]*docker.RegistryAuth{}
	if len(hosts) == 0 {
		return creds, nil
	}
	if o.RegistryUser != "" {
		creds[hosts[0]] = &docker.RegistryAuth{Host: hosts[0], Username: o.RegistryUser, Password: o.RegistryPassword}
		return creds, nil
	}
	if configPath == "" {
		return creds, nil
	}
	cfg, err := readDockerConfig(configPath)
	if err != nil || cfg == nil {
		return creds, err
	}
	for _, h := range hosts {
		c, err := cfg.lookup(h)
		if err != nil {
			return nil, err
		}
		if c != nil {
			creds[h] = c
		}
	}
	return creds, nil
}

// dockerConfigFile es lo que install lee del config.json de Docker.
type dockerConfigFile struct {
	path        string
	Auths       map[string]dockerAuthEntry `json:"auths"`
	CredsStore  string                     `json:"credsStore"`
	CredHelpers map[string]string          `json:"credHelpers"`
}

type dockerAuthEntry struct {
	Auth     string `json:"auth"` // base64 de usuario:contraseña
	Username string `json:"username"`
	Password string `json:"password"`
}

// readDockerConfig lee el config.json; nil si no existe (no se montó).
func readDockerConfig(path string) (*dockerConfigFile, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leyendo el config.json de Docker (%s): %w", path, err)
	}
	cfg := &dockerConfigFile{path: path}
	// El error de encoding/json puede citar un pedazo del archivo: no se muestra.
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("el config.json de Docker (%s) no es un JSON válido", path)
	}
	return cfg, nil
}

// normalizeRegistry lleva una clave de auths o de credHelpers al host de
// reference.Domain: sin esquema ni ruta ("https://index.docker.io/v1/" es
// docker.io, como lo guarda docker login para Docker Hub).
func normalizeRegistry(key string) string {
	key = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(key), "https://"), "http://")
	key, _, _ = strings.Cut(key, "/")
	switch key {
	case "index.docker.io", "registry-1.docker.io", "registry.hub.docker.com":
		return "docker.io"
	}
	return key
}

// lookup busca la credencial de host. Si docker login la guardó en un
// helper (credsStore o credHelpers), el helper no se puede correr dentro
// del contenedor: error que pide los flags.
func (c *dockerConfigFile) lookup(host string) (*docker.RegistryAuth, error) {
	bad := func(what string) error {
		return fmt.Errorf("el config.json de Docker (%s) tiene una credencial de %s que no sirve (%s): rehaz docker login %s o usa --registry-user y --registry-password-stdin",
			c.path, host, what, host)
	}
	loggedIn := false
	// En orden, por si hay dos claves del mismo host ("ghcr.io" y "https://ghcr.io").
	for _, key := range slices.Sorted(maps.Keys(c.Auths)) {
		e := c.Auths[key]
		if normalizeRegistry(key) != host {
			continue
		}
		loggedIn = true
		user, pass := e.Username, e.Password
		if e.Auth != "" {
			raw, err := base64.StdEncoding.DecodeString(e.Auth)
			if err != nil {
				return nil, bad("auth no es base64")
			}
			var ok bool
			user, pass, ok = strings.Cut(string(raw), ":")
			if !ok {
				return nil, bad("auth no es usuario:contraseña")
			}
		}
		if user == "" && pass == "" {
			continue // una entrada vacía: la credencial está en el helper
		}
		if checkRegistryUser(user) != nil || checkRegistryPassword(pass) != nil {
			return nil, bad("el usuario o la contraseña están vacíos, son muy largos o tienen caracteres de control")
		}
		return &docker.RegistryAuth{Host: host, Username: user, Password: pass}, nil
	}
	helper := ""
	for _, key := range slices.Sorted(maps.Keys(c.CredHelpers)) {
		if normalizeRegistry(key) == host && helper == "" {
			helper = c.CredHelpers[key]
		}
	}
	if helper == "" && loggedIn {
		helper = c.CredsStore
	}
	if helper != "" {
		return nil, fmt.Errorf("el config.json de Docker (%s) guarda la credencial de %s en el helper docker-credential-%s, que no se puede correr dentro del contenedor de install: "+
			"pasa la credencial con --registry-user <usuario> y --registry-password-stdin (docker run -i, con la contraseña o el token en stdin)", c.path, host, helper)
	}
	return nil, nil
}
