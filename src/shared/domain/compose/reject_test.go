package compose

import (
	"context"
	"strings"
	"testing"
)

// TestRejects tiene un caso por cada rechazo de §6.3 del diseño: cada fila
// prueba una clave sola (las combinaciones las rechaza compose-go con su
// propio texto) y un pedazo del mensaje esperado, que incluye la ruta.
func TestRejects(t *testing.T) {
	const (
		publish  = "publicar puertos no está soportado"
		build    = "construir imágenes no está soportado"
		host     = "montar el host no está soportado"
		topVol   = "un volumen de primer nivel solo acepta labels"
		devices  = "dispositivos"
		priv     = "escala privilegios"
		files    = "archivos del servidor de muelle"
		secrets  = "secrets y configs no están soportados"
		network  = "red del stage"
		name     = "el nombre del proyecto lo fija muelle"
		logging  = "solo se aceptan los drivers json-file, local y none"
		dnsOpt   = "docker stack deploy no acepta dns_opt"
		jobs     = "los jobs no están soportados"
		socket   = "socket de Docker"
		generic  = "no está soportado: muelle solo acepta las claves de compose que conoce (§6.3)"
		reserved = "está reservada"
	)
	vol := "volumes:\n  data: {}\n"
	cases := []struct{ name, src, want string }{
		// Publicar o construir.
		{"ports", web("    ports: [\"80:80\"]\n"), "services.web.ports: " + publish},
		{"build", web("    build: .\n"), "services.web.build: " + build},

		// Montar el host.
		{"docker.sock", web("    volumes: [\"/var/run/docker.sock:/var/run/docker.sock\"]\n"), "services.web.volumes.bind: " + host},
		{"bind relativo", web("    volumes: [\"./x:/x\"]\n"), "services.web.volumes.bind: " + host},
		{"type bind", web("    volumes:\n      - {type: bind, source: /etc, target: /x}\n"), "services.web.volumes: " + host},
		{"type npipe", web("    volumes:\n      - {type: npipe, source: x, target: /x}\n"), "services.web.volumes: " + host},
		{"type cluster", web("    volumes:\n      - {type: cluster, source: x, target: /x}\n"), "services.web.volumes: " + host},
		{"type image", web("    volumes:\n      - {type: image, source: alpine, target: /x}\n"), "services.web.volumes: " + host},
		{"volumen anónimo", web("    volumes:\n      - {type: volume, target: /x}\n"), "services.web.volumes: un volumen sin source (anónimo)"},
		{"volumen anónimo corto", web("    volumes: [/x]\n"), "services.web.volumes: un volumen sin source (anónimo)"},
		{"volumen sin declarar", web("    volumes: [\"otro:/x\"]\n") + vol, `services.web.volumes: el volumen "otro" no está en los volumes de primer nivel`},
		{"subpath", web("    volumes:\n      - {type: volume, source: data, target: /x, volume: {subpath: a}}\n") + vol, "services.web.volumes.volume.subpath: "},
		{"driver", web("") + "volumes:\n  data:\n    driver: local\n", "volumes.data.driver: " + topVol},
		{"driver_opts", web("") + "volumes:\n  data:\n    driver_opts: {type: none, o: bind, device: /var/run}\n", "volumes.data.driver_opts: " + topVol},
		{"external", web("") + "volumes:\n  data:\n    external: true\n", "volumes.data.external: " + topVol},
		{"name de volumen", web("") + "volumes:\n  data:\n    name: muelle_data\n", "volumes.data.name: " + topVol},
		{"devices", web("    devices: [\"/dev/sda:/dev/sda\"]\n"), "services.web.devices: " + devices},
		{"device_cgroup_rules", web("    device_cgroup_rules: [\"c 1:3 mr\"]\n"), "services.web.device_cgroup_rules: " + devices},
		{"volumes_from", web("    volumes_from: [otro]\n"), "services.web.volumes_from: "},

		// Escalar privilegios o salir del aislamiento.
		{"privileged", web("    privileged: true\n"), "services.web.privileged: " + priv},
		{"cap_add", web("    cap_add: [SYS_ADMIN]\n"), "services.web.cap_add: " + priv},
		{"security_opt", web("    security_opt: [\"seccomp:unconfined\"]\n"), "services.web.security_opt: " + priv},
		{"sysctls", web("    sysctls: {net.core.somaxconn: 1024}\n"), "services.web.sysctls: " + priv},
		{"pid", web("    pid: host\n"), "services.web.pid: " + priv},
		{"ipc", web("    ipc: host\n"), "services.web.ipc: " + priv},
		{"uts", web("    uts: host\n"), "services.web.uts: " + priv},
		{"userns_mode", web("    userns_mode: host\n"), "services.web.userns_mode: " + priv},
		{"cgroup", web("    cgroup: host\n"), "services.web.cgroup: " + priv},
		{"cgroup_parent", web("    cgroup_parent: x\n"), "services.web.cgroup_parent: " + priv},
		{"runtime", web("    runtime: runc\n"), "services.web.runtime: " + priv},
		{"isolation", web("    isolation: default\n"), "services.web.isolation: " + priv},
		{"credential_spec", web("    credential_spec: {file: x}\n"), "services.web.credential_spec: " + priv},

		// Leer archivos de muelle.
		{"secrets de primer nivel", web("") + "secrets:\n  s:\n    file: /run/secrets/muelle_master_key\n", "secrets: " + secrets},
		{"secrets del servicio", web("    secrets: [s]\n"), "services.web.secrets: " + secrets},
		{"configs de primer nivel", web("") + "configs:\n  c:\n    file: /data/muelle.db\n", "configs: " + secrets},
		{"configs del servicio", web("    configs: [c]\n"), "services.web.configs: " + secrets},
		{"env_file", web("    env_file: .env\n"), "services.web.env_file: " + files},
		{"label_file", web("    label_file: labels\n"), "services.web.label_file: " + files},
		{"extends", web("    extends: {file: base.yaml, service: x}\n"), "services.web.extends: " + files},
		{"include", "include: [otro.yaml]\n" + web(""), "include: " + files},

		// Redes y nombres.
		{"container_name", web("    container_name: otro\n"), "services.web.container_name: " + network},
		{"network_mode", web("    network_mode: host\n"), "services.web.network_mode: " + network},
		{"links", web("    links: [db]\n"), "services.web.links: " + network},
		{"external_links", web("    external_links: [db]\n"), "services.web.external_links: " + network},
		{"networks del servicio", web("    networks: [front]\n"), "services.web.networks: " + network},
		{"networks de primer nivel", web("") + "networks:\n  front: {}\n", "networks: " + network},
		{"name de primer nivel", "name: otro\n" + web(""), "name: " + name},

		// Logging.
		{"syslog", web("    logging: {driver: syslog}\n"), "services.web.logging.driver: " + logging},
		{"gelf", web("    logging: {driver: gelf}\n"), "services.web.logging.driver: " + logging},
		{"fluentd", web("    logging: {driver: fluentd}\n"), "services.web.logging.driver: " + logging},

		// Lo que el CLI no soporta o la v1 no tiene.
		{"dns_opt", web("    dns_opt: [use-vc]\n"), "services.web.dns_opt: " + dnsOpt},
		{"replicated-job", web("    deploy: {mode: replicated-job}\n"), "services.web.deploy.mode: " + jobs},
		{"global-job", web("    deploy: {mode: global-job}\n"), "services.web.deploy.mode: " + jobs},
		{"pids", web("    deploy: {resources: {limits: {pids: 100}}}\n"), "services.web.deploy.resources.limits.pids: "},
		{"reservations.devices", web("    deploy:\n      resources:\n        reservations:\n          devices: [{capabilities: [gpu]}]\n"), "services.web.deploy.resources.reservations.devices: " + devices},
		{"use_api_socket", web("    use_api_socket: true\n"), "services.web.use_api_socket: " + socket},
		{"models del servicio", web("    models: [m]\n"), "services.web.models: "},
		{"models de primer nivel", web("") + "models:\n  m:\n    model: ai/smollm2\n", "models: "},
		{"gpus", web("    gpus: all\n"), "services.web.gpus: " + devices},
		{"develop", web("    develop:\n      watch: [{path: ., action: rebuild}]\n"), "services.web.develop: "},
		{"pre_start", web("    pre_start: [{image: alpine}]\n"), "services.web.pre_start: "},
		{"jobs", web("") + "jobs:\n  j:\n    image: alpine\n    triggers: {manual: true}\n", "jobs: "},

		// Una clave inventada la rechaza el esquema de compose-go, con el
		// mismo formato.
		{"foo en el servicio", web("    foo: 1\n"), "services.web.foo: " + generic},
		{"foo de primer nivel", "foo: 1\n" + web(""), "foo: " + generic},
		{"foo en deploy", web("    deploy: {foo: 1}\n"), "services.web.deploy.foo: " + generic},
		{"foo en healthcheck", web("    healthcheck: {foo: 1}\n"), "services.web.healthcheck.foo: " + generic},
	}

	// Las etiquetas reservadas, en las tres sintaxis y en los tres lugares.
	for _, key := range []string{"muelle.app", "com.docker.stack.namespace"} {
		for form, value := range map[string]string{
			"mapa":           "{" + key + ": x}",
			"lista con =":    "[\"" + key + "=x\"]",
			"lista sin =":    "[\"" + key + "\"]",
			"lista con = ''": "[\"" + key + "=\"]",
		} {
			cases = append(cases,
				struct{ name, src, want string }{key + " en labels (" + form + ")",
					web("    labels: " + value + "\n"), "services.web.labels: la etiqueta \"" + key + "\" " + reserved},
				struct{ name, src, want string }{key + " en deploy.labels (" + form + ")",
					web("    deploy:\n      labels: " + value + "\n"), "services.web.deploy.labels: la etiqueta \"" + key + "\" " + reserved},
				struct{ name, src, want string }{key + " en un volumen (" + form + ")",
					web("") + "volumes:\n  data:\n    labels: " + value + "\n", "volumes.data.labels: la etiqueta \"" + key + "\" " + reserved},
			)
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// c.want es "<ruta>: <pedazo del motivo>": el mensaje empieza con
			// la ruta y contiene el pedazo.
			path, frag, _ := strings.Cut(c.want, ": ")
			_, err := Validate(c.src)
			ve := wantInvalid(t, err, frag)
			if !strings.HasPrefix(ve.Error(), "compose_yaml: "+path+": ") {
				t.Fatalf("el error %q no empieza con la ruta %s", ve.Error(), path)
			}
			// El render rechaza lo mismo.
			_, err = Load(context.Background(), c.src, "p_s_a", map[string]string{})
			if ve2 := wantInvalid(t, err); ve2.Error() != ve.Error() {
				t.Fatalf("Load: %q, Validate: %q", ve2.Error(), ve.Error())
			}
		})
	}
}

func TestExtensionsPass(t *testing.T) {
	src := "x-top: {a: 1}\n" + web("    x-cualquier: {b: [1, 2]}\n    deploy:\n      x-otra: 1\n")
	if _, err := Validate(src); err != nil {
		t.Fatal(err)
	}
	f, err := Load(context.Background(), src, "p_s_a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Services["web"].Image != "nginx" {
		t.Fatalf("%+v", f)
	}
}

// TestSchemaMessage fija los textos de compose-go v2.16.1 que reconoce
// schemaMessage: si cambian al subir la versión, este test lo dice.
func TestSchemaMessage(t *testing.T) {
	cases := []struct{ src, want string }{
		{web("    foo: 1\n"), "compose_yaml: services.web.foo: " + "no está soportado"},
		{"foo: 1\nbar: 2\nservices:\n  web:\n    image: nginx\n", "compose_yaml: bar: no está soportado"},
		{web("    deploy: {zeta: 1, alfa: 2}\n"), "compose_yaml: services.web.deploy.alfa: no está soportado"},
		{web("    ports: [1]\n    foo: 1\n"), "compose_yaml: services.web.foo: "},
	}
	for _, c := range cases {
		_, err := Validate(c.src)
		ve := wantInvalid(t, err)
		if got := ve.Error(); len(got) < len(c.want) || got[:len(c.want)] != c.want {
			t.Errorf("%q:\n got %q\nwant prefijo %q", c.src, got, c.want)
		}
	}
	// Los textos crudos de compose-go.
	for raw, want := range map[string]string{
		"validating compose.yaml: services.web.foo false schema":                               "compose_yaml: services.web.foo: no está soportado",
		"validating compose.yaml:  additional properties 'foo', 'bar' not allowed":             "compose_yaml: bar: no está soportado",
		"validating compose.yaml: services.web.deploy additional properties 'foo' not allowed": "compose_yaml: services.web.deploy.foo: no está soportado",
		"validating compose.yaml: services.web.ports false schema":                             "compose_yaml: services.web.ports: " + "publicar puertos",
		"validating compose.yaml: services.web.image must be a string":                         "compose_yaml: el compose no es válido: validating compose.yaml: services.web.image must be a string",
		"failed to parse compose.yaml: yaml: line 1:\nunexpected":                              "compose_yaml: el compose no es válido: failed to parse compose.yaml: yaml: line 1: unexpected",
	} {
		ve := wantInvalid(t, schemaMessage(errorString(raw)))
		if got := ve.Error(); len(got) < len(want) || got[:len(want)] != want {
			t.Errorf("%q:\n got %q\nwant prefijo %q", raw, got, want)
		}
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
