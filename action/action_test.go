// Package action solo tiene tests: corren el deploy.sh real de la GitHub Action
// (diseño §14) con bash, curl y jq contra un muelle falso de httptest. Se saltan
// si falta alguna de esas herramientas.
package action

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testToken = "mk_0123456789abcdefghij"
	appPath   = "/api/v1/projects/auditfy/stages/staging/apps/api"
	statusURL = appPath + "/deploys/145"
)

func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("falta %s: los tests de la action corren deploy.sh con bash, curl y jq", tool)
		}
	}
}

// step es una respuesta del muelle falso.
type step struct {
	code   int
	body   string
	header map[string]string
	hijack bool // cierra la conexión sin responder
}

func jsonStep(code int, v any) step {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return step{code: code, body: string(b)}
}

func errStep(code int, errCode, message string) step {
	return jsonStep(code, map[string]any{"error": map[string]string{"code": errCode, "message": message}})
}

func logStep(chunk string, next int, status string, supersededBy any) step {
	return jsonStep(http.StatusOK, map[string]any{
		"chunk": chunk, "next_offset": next, "status": status, "superseded_by": supersededBy,
	})
}

func createdStep(statusURL string) step {
	return jsonStep(http.StatusAccepted, map[string]any{"id": 145, "status": "queued", "status_url": statusURL})
}

func deployStep(status string, deployErr any) step {
	return jsonStep(http.StatusOK, map[string]any{
		"id": 145, "status": status, "trigger": "api", "error": deployErr, "superseded_by": nil,
	})
}

func withHeader(s step, k, v string) step {
	s.header = map[string]string{k: v}
	return s
}

// routes son las respuestas del muelle falso por ruta, en orden; la última se
// repite. Una ruta sin respuestas usa la de siempre: PUT env 204, POST 202 con
// statusURL, el log vacío y succeeded, y el deploy succeeded.
type routes struct {
	put, post, log, get []step
}

type request struct {
	method, uri, auth, body string
}

type fakeMuelle struct {
	*httptest.Server
	routes routes

	mu     sync.Mutex
	reqs   []request
	counts map[string]int
}

func newFake(t *testing.T, r routes) *fakeMuelle {
	t.Helper()
	if r.put == nil {
		r.put = []step{{code: http.StatusNoContent}}
	}
	if r.post == nil {
		r.post = []step{createdStep(statusURL)}
	}
	if r.log == nil {
		r.log = []step{logStep("", 0, "succeeded", nil)}
	}
	if r.get == nil {
		r.get = []step{deployStep("succeeded", nil)}
	}
	f := &fakeMuelle{routes: r, counts: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeMuelle) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	route, seq := "", []step{errStep(http.StatusNotFound, "no_route", "la ruta no existe")}
	deploys := appPath + "/deploys/"
	switch {
	case r.Method == http.MethodPut && r.URL.Path == appPath+"/env":
		route, seq = "put", f.routes.put
	case r.Method == http.MethodPost && r.URL.Path == appPath+"/deploys":
		route, seq = "post", f.routes.post
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, deploys) && strings.HasSuffix(r.URL.Path, "/log"):
		route, seq = "log", f.routes.log
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, deploys):
		route, seq = "get", f.routes.get
	}

	f.mu.Lock()
	f.reqs = append(f.reqs, request{r.Method, r.RequestURI, r.Header.Get("Authorization"), string(body)})
	n := f.counts[route]
	f.counts[route]++
	f.mu.Unlock()

	s := seq[min(n, len(seq)-1)]
	if s.hijack {
		if conn, _, err := http.NewResponseController(w).Hijack(); err == nil {
			_ = conn.Close()
		}
		return
	}
	for k, v := range s.header {
		w.Header().Set(k, v)
	}
	if s.body != "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(s.code)
	_, _ = io.WriteString(w, s.body)
}

func (f *fakeMuelle) requests() []request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

// count cuenta las peticiones con ese método.
func (f *fakeMuelle) count(method string) int {
	n := 0
	for _, r := range f.requests() {
		if r.method == method {
			n++
		}
	}
	return n
}

type result struct {
	stdout, stderr string
	outputs        map[string]string
	rawOutputs     string
	code           int
	elapsed        time.Duration
}

// errors son las líneas ::error:: de stdout.
func (r result) errors() []string { return commandLines(r.stdout, "::error::") }

func (r result) warnings() []string { return commandLines(r.stdout, "::warning::") }

func commandLines(out, prefix string) []string {
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(l, prefix) {
			lines = append(lines, l)
		}
	}
	return lines
}

// lastError es la última línea ::error::, o falla si no hay.
func (r result) lastError(t *testing.T) string {
	t.Helper()
	errs := r.errors()
	if len(errs) == 0 {
		t.Fatalf("sin ::error::\nstdout:\n%s\nstderr:\n%s", r.stdout, r.stderr)
	}
	return errs[len(errs)-1]
}

// runAction corre deploy.sh como lo corre action.yml: las entradas como INPUT_*
// (con sus defaults), GITHUB_OUTPUT en un archivo, RUNNER_TEMP en un directorio
// que al terminar tiene que quedar vacío (el archivo del token se borra), y los
// tiempos de prueba (polling de 10 ms, sin espera entre los PUT).
func runAction(t *testing.T, url string, inputs map[string]string) result {
	t.Helper()
	requireTools(t)
	in := map[string]string{
		"url": url, "token": testToken, "app": "auditfy/staging/api",
		"image": "", "images-json": "", "env-json": "", "env-exclude": "",
		"env-secret": "true", "timeout": "600",
	}
	maps.Copy(in, inputs)

	outDir, runnerTemp := t.TempDir(), t.TempDir()
	outFile := filepath.Join(outDir, "github_output")
	if err := os.WriteFile(outFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + outDir,
		"GITHUB_OUTPUT=" + outFile,
		"RUNNER_TEMP=" + runnerTemp,
		"MUELLE_ACTION_TEST_POLL=0.01",
		"MUELLE_ACTION_TEST_RETRY_DELAYS=0 0",
	}
	for k, v := range in {
		env = append(env, "INPUT_"+strings.ToUpper(strings.ReplaceAll(k, "-", "_"))+"="+v)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "deploy.sh")
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	res := result{stdout: stdout.String(), stderr: stderr.String(), elapsed: time.Since(start)}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		res.code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("deploy.sh: %v", err)
	}

	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	res.rawOutputs = string(raw)
	res.outputs = map[string]string{}
	for l := range strings.SplitSeq(strings.TrimSuffix(res.rawOutputs, "\n"), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			res.outputs[k] = v
		}
	}
	if left, _ := os.ReadDir(runnerTemp); len(left) > 0 {
		t.Errorf("deploy.sh dejó %d archivos en RUNNER_TEMP (el del token tiene que borrarse)", len(left))
	}
	return res
}

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("el cuerpo %q no es JSON: %v", body, err)
	}
	return m
}

func mustEqualJSON(t *testing.T, got string, want any) {
	t.Helper()
	g := decodeBody(t, got)
	w, _ := json.Marshal(want)
	var wm map[string]any
	_ = json.Unmarshal(w, &wm)
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(wm)
	if string(gb) != string(wb) {
		t.Errorf("cuerpo = %s, quiero %s", gb, wb)
	}
}

// yamlBlock lee un bloque de primer nivel de action.yml (inputs: u outputs:) como
// nombre → clave → valor, sin comillas. Alcanza para el formato de action.yml.
func yamlBlock(t *testing.T, lines []string, top string) map[string]map[string]string {
	t.Helper()
	block := map[string]map[string]string{}
	in, name := false, ""
	entry := regexp.MustCompile(`^  ([a-z-]+):\s*$`)
	field := regexp.MustCompile(`^    ([a-z]+):\s*(.*)$`)
	for _, l := range lines {
		switch {
		case l == top+":":
			in = true
			continue
		case !in, strings.TrimSpace(l) == "", strings.HasPrefix(strings.TrimSpace(l), "#"):
			continue
		case !strings.HasPrefix(l, " "):
			return block
		}
		if m := entry.FindStringSubmatch(l); m != nil {
			name = m[1]
			block[name] = map[string]string{}
		} else if m := field.FindStringSubmatch(l); m != nil && name != "" {
			v := m[2]
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = v[1 : len(v)-1]
			}
			block[name][m[1]] = v
		}
	}
	if !in {
		t.Fatalf("action.yml no tiene %s:", top)
	}
	return block
}

func TestActionYAMLPassesInputsByEnv(t *testing.T) {
	raw, err := os.ReadFile("action.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	lines := strings.Split(text, "\n")

	if !regexp.MustCompile(`(?m)^  using: composite$`).MatchString(text) {
		t.Error("action.yml no es una composite action (using: composite)")
	}

	inputs := yamlBlock(t, lines, "inputs")
	wantDefaults := map[string]string{
		"url": "", "token": "", "app": "",
		"image": "", "images-json": "", "env-json": "", "env-exclude": "",
		"env-secret": "true", "timeout": "600",
	}
	if got := slices.Sorted(maps.Keys(inputs)); !slices.Equal(got, slices.Sorted(maps.Keys(wantDefaults))) {
		t.Errorf("entradas = %v, quiero %v", got, slices.Sorted(maps.Keys(wantDefaults)))
	}
	for name, def := range wantDefaults {
		in, ok := inputs[name]
		if !ok {
			continue
		}
		required := name == "url" || name == "token" || name == "app"
		if in["required"] != map[bool]string{true: "true", false: "false"}[required] {
			t.Errorf("%s: required = %q", name, in["required"])
		}
		if !required {
			if d, ok := in["default"]; !ok || d != def {
				t.Errorf("%s: default = %q (presente: %v), quiero %q", name, d, ok, def)
			}
		}
		if in["description"] == "" {
			t.Errorf("%s: sin description", name)
		}
		envLine := "INPUT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + ": ${{ inputs." + name + " }}"
		if !regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(envLine) + `$`).MatchString(text) {
			t.Errorf("la entrada %s no llega por env como %q", name, envLine)
		}
	}

	outputs := yamlBlock(t, lines, "outputs")
	for _, name := range []string{"deploy-id", "status"} {
		want := "${{ steps.deploy.outputs." + name + " }}"
		if outputs[name]["value"] != want {
			t.Errorf("salida %s: value = %q, quiero %q", name, outputs[name]["value"], want)
		}
	}
	if len(outputs) != 2 {
		t.Errorf("salidas = %v, quiero deploy-id y status", slices.Sorted(maps.Keys(outputs)))
	}

	// Una expresión solo puede estar en el env: del paso o en el value: de una
	// salida: dentro de un run:, una entrada podría inyectar comandos.
	envExpr := regexp.MustCompile(`^\s+INPUT_[A-Z_]+: \$\{\{ inputs\.[a-z-]+ \}\}$`)
	outExpr := regexp.MustCompile(`^\s+value: \$\{\{ steps\.deploy\.outputs\.[a-z-]+ \}\}$`)
	runs := 0
	for _, l := range lines {
		if strings.Contains(l, "${{") && !envExpr.MatchString(l) && !outExpr.MatchString(l) {
			t.Errorf("expresión fuera del env: del paso: %q", l)
		}
		if strings.HasPrefix(strings.TrimSpace(l), "run:") {
			runs++
			if strings.TrimSpace(l) != `run: bash "$GITHUB_ACTION_PATH/deploy.sh"` {
				t.Errorf("run: = %q", strings.TrimSpace(l))
			}
		}
	}
	if runs != 1 {
		t.Errorf("%d líneas run:, quiero 1", runs)
	}
	if !regexp.MustCompile(`(?m)^\s+shell: bash$`).MatchString(text) {
		t.Error("el paso no corre con shell: bash")
	}
}

func TestDeploySucceeded(t *testing.T) {
	t.Parallel()
	l1 := "2026-10-15T10:00:00.000Z resolviendo imágenes\n"
	l2 := "2026-10-15T10:00:05.000Z servicio auditfy_staging_api_api actualizado\n"
	f := newFake(t, routes{log: []step{
		logStep(l1, len(l1), "running", nil),
		logStep(l2, len(l1+l2), "running", nil),
		logStep("", len(l1+l2), "succeeded", nil),
	}})
	image := "ghcr.io/aumi-app/backend-app:sha-3f9c2e1"
	res := runAction(t, f.URL, map[string]string{"image": image})

	if res.code != 0 {
		t.Fatalf("código %d, quiero 0\nstdout:\n%s\nstderr:\n%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, l1+l2) {
		t.Errorf("stdout no trae el log:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, "::add-mask::"+testToken+"\n") {
		t.Error("el token no se enmascara (::add-mask::)")
	}
	if res.outputs["deploy-id"] != "145" || res.outputs["status"] != "succeeded" {
		t.Errorf("salidas = %v", res.outputs)
	}
	reqs := f.requests()
	wantURIs := []string{
		"POST " + appPath + "/deploys",
		"GET " + statusURL + "/log?offset=0",
		"GET " + statusURL + "/log?offset=" + strconv.Itoa(len(l1)),
		"GET " + statusURL + "/log?offset=" + strconv.Itoa(len(l1+l2)),
	}
	var got []string
	for _, r := range reqs {
		got = append(got, r.method+" "+r.uri)
		if r.auth != "Bearer "+testToken {
			t.Errorf("%s %s: Authorization = %q", r.method, r.uri, r.auth)
		}
	}
	if !slices.Equal(got, wantURIs) {
		t.Errorf("peticiones:\n%s\nquiero:\n%s", strings.Join(got, "\n"), strings.Join(wantURIs, "\n"))
	}
	if len(reqs) > 0 {
		mustEqualJSON(t, reqs[0].body, map[string]any{"image": image})
	}
}

func TestDeployBodies(t *testing.T) {
	t.Parallel()
	t.Run("sin imagen", func(t *testing.T) {
		f := newFake(t, routes{})
		res := runAction(t, f.URL, nil)
		if res.code != 0 {
			t.Fatalf("código %d\n%s", res.code, res.stdout)
		}
		mustEqualJSON(t, f.requests()[0].body, map[string]any{})
	})
	t.Run("images-json", func(t *testing.T) {
		f := newFake(t, routes{})
		images := map[string]any{"api": "ghcr.io/x/api:1", "worker": "ghcr.io/x/worker:1"}
		b, _ := json.Marshal(images)
		res := runAction(t, f.URL, map[string]string{"images-json": string(b)})
		if res.code != 0 {
			t.Fatalf("código %d\n%s", res.code, res.stdout)
		}
		mustEqualJSON(t, f.requests()[0].body, map[string]any{"images": images})
	})
	t.Run("image e images-json", func(t *testing.T) {
		f := newFake(t, routes{})
		res := runAction(t, f.URL, map[string]string{"image": "ghcr.io/x/api:1", "images-json": `{"api":"ghcr.io/x/api:1"}`})
		if res.code != 1 || len(res.errors()) == 0 {
			t.Errorf("código %d, errores %v", res.code, res.errors())
		}
		if n := len(f.requests()); n != 0 {
			t.Errorf("%d peticiones, quiero 0", n)
		}
	})
}

func TestDeployFailed(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{
		log: []step{logStep("boom\n", 5, "failed", nil), logStep("", 5, "failed", nil)},
		get: []step{deployStep("failed", "la imagen ghcr.io/x/api:1 no existe")},
	})
	res := runAction(t, f.URL, nil)
	if res.code != 1 {
		t.Fatalf("código %d, quiero 1\n%s", res.code, res.stdout)
	}
	if e := res.lastError(t); !strings.Contains(e, "la imagen ghcr.io/x/api:1 no existe") {
		t.Errorf("error = %q", e)
	}
	if res.outputs["status"] != "failed" || res.outputs["deploy-id"] != "145" {
		t.Errorf("salidas = %v", res.outputs)
	}
	reqs := f.requests()
	if last := reqs[len(reqs)-1]; last.method != http.MethodGet || last.uri != statusURL {
		t.Errorf("última petición = %s %s, quiero GET %s", last.method, last.uri, statusURL)
	}
}

func TestDeploySuperseded(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{log: []step{logStep("", 0, "superseded", 146)}})
	res := runAction(t, f.URL, nil)
	if res.code != 0 {
		t.Fatalf("código %d, quiero 0\n%s", res.code, res.stdout)
	}
	if w := res.warnings(); len(w) != 1 || !strings.Contains(w[0], "#146") {
		t.Errorf("avisos = %v, quiero uno con #146", w)
	}
	if res.outputs["status"] != "superseded" {
		t.Errorf("salidas = %v", res.outputs)
	}
}

func TestDeployTimeout(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{log: []step{logStep("", 0, "running", nil)}})
	res := runAction(t, f.URL, map[string]string{"timeout": "1"})
	if res.code != 1 {
		t.Fatalf("código %d, quiero 1\n%s", res.code, res.stdout)
	}
	if e := res.lastError(t); !strings.Contains(e, "timeout") {
		t.Errorf("error = %q", e)
	}
	if res.outputs["status"] != "running" {
		t.Errorf("salidas = %v", res.outputs)
	}
	if res.elapsed < time.Second || res.elapsed > 10*time.Second {
		t.Errorf("tardó %s con timeout 1: quiero al menos 1 s", res.elapsed)
	}
}

func TestStopCommandsWrapTheLog(t *testing.T) {
	t.Parallel()
	appLine := "::error::de la app"
	stopRe := regexp.MustCompile(`^::stop-commands::([0-9a-f]{32})$`)
	var tokens []string
	for range 2 {
		f := newFake(t, routes{
			log: []step{logStep(appLine+"\n", len(appLine)+1, "failed", nil), logStep("", len(appLine)+1, "failed", nil)},
			get: []step{deployStep("failed", "falló el healthcheck")},
		})
		res := runAction(t, f.URL, nil)
		lines := strings.Split(res.stdout, "\n")
		stop, app, end, own := -1, -1, -1, -1
		tok := ""
		for i, l := range lines {
			switch {
			case stopRe.MatchString(l):
				stop, tok = i, stopRe.FindStringSubmatch(l)[1]
			case l == appLine:
				app = i
			case tok != "" && l == "::"+tok+"::":
				end = i
			case strings.HasPrefix(l, "::error::") && strings.Contains(l, "falló el healthcheck"):
				own = i
			}
		}
		if stop < 0 || app < 0 || end < 0 || own < 0 || !(stop < app && app < end && end < own) {
			t.Fatalf("orden stop=%d app=%d cierre=%d error=%d:\n%s", stop, app, end, own, res.stdout)
		}
		tokens = append(tokens, tok)
	}
	if tokens[0] == tokens[1] {
		t.Error("el token de ::stop-commands:: se repite entre corridas")
	}
}

func TestLogWithoutTrailingNewline(t *testing.T) {
	t.Parallel()
	// El contrato manda líneas completas, pero si un chunk no termina en \n, el
	// cierre (::<tok>::) tiene que ir en su propia línea o el runner no lo lee.
	f := newFake(t, routes{log: []step{logStep("sin salto", 9, "running", nil), logStep("", 9, "succeeded", nil)}})
	res := runAction(t, f.URL, nil)
	if res.code != 0 {
		t.Fatalf("código %d\n%s", res.code, res.stdout)
	}
	if !regexp.MustCompile(`(?m)^sin salto\n::[0-9a-f]{32}::$`).MatchString(res.stdout) {
		t.Errorf("el cierre no va en su propia línea:\n%s", res.stdout)
	}
}

func TestEnvPutFiltersExcluded(t *testing.T) {
	t.Parallel()
	envJSON := `{"github_token":"ghs_x","MUELLE_URL":"https://m.example.com","Muelle_Token":"mk_y",` +
		`"EXPORT_PASSPHRASE":"frase","DB_URL":"postgres://db"}`
	for _, secret := range []bool{true, false} {
		f := newFake(t, routes{})
		res := runAction(t, f.URL, map[string]string{
			"env-json": envJSON, "env-exclude": " export_passphrase ,,", "env-secret": map[bool]string{true: "true", false: "false"}[secret],
		})
		if res.code != 0 {
			t.Fatalf("código %d\n%s", res.code, res.stdout)
		}
		reqs := f.requests()
		if len(reqs) < 2 || reqs[0].method != http.MethodPut || reqs[0].uri != appPath+"/env" || reqs[1].method != http.MethodPost {
			t.Fatalf("peticiones = %v, quiero el PUT …/env y después el POST", reqs)
		}
		mustEqualJSON(t, reqs[0].body, map[string]any{"vars": map[string]string{"DB_URL": "postgres://db"}, "secret": secret})
		if reqs[0].auth != "Bearer "+testToken {
			t.Errorf("PUT: Authorization = %q", reqs[0].auth)
		}
	}
}

func TestEnvPutRetries(t *testing.T) {
	t.Parallel()
	envJSON := `{"DB_URL":"postgres://db"}`
	unavailable := errStep(http.StatusServiceUnavailable, "docker_unavailable", "Docker no responde")
	limited := errStep(http.StatusTooManyRequests, "rate_limited", "demasiadas peticiones")
	ok := step{code: http.StatusNoContent}
	cases := []struct {
		name       string
		put        []step
		code       int
		puts       int
		posts      int
		minElapsed time.Duration
		maxElapsed time.Duration
		errHas     string
	}{
		{name: "503 dos veces y después 204", put: []step{unavailable, unavailable, ok}, code: 0, puts: 3, posts: 1},
		{name: "tres 503", put: []step{unavailable}, code: 1, puts: 3, posts: 0, errHas: "docker_unavailable"},
		{name: "422 no se reintenta", put: []step{errStep(http.StatusUnprocessableEntity, "invalid", "vars: la clave 1X no sirve")}, code: 1, puts: 1, errHas: "vars: la clave 1X no sirve"},
		{name: "429 con Retry-After 1", put: []step{withHeader(limited, "Retry-After", "1"), ok}, code: 0, puts: 2, posts: 1, minElapsed: time.Second},
		{name: "429 con Retry-After 61", put: []step{withHeader(limited, "Retry-After", "61")}, code: 1, puts: 1, errHas: "rate_limited", maxElapsed: 10 * time.Second},
		{name: "429 sin Retry-After", put: []step{limited, ok}, code: 0, puts: 2, posts: 1, maxElapsed: 10 * time.Second},
		{name: "429 con una fecha HTTP", put: []step{withHeader(limited, "Retry-After", "Wed, 21 Oct 2026 07:28:00 GMT"), ok}, code: 0, puts: 2, posts: 1, maxElapsed: 10 * time.Second},
		{name: "tres 429", put: []step{limited}, code: 1, puts: 3, errHas: "rate_limited"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake(t, routes{put: c.put})
			res := runAction(t, f.URL, map[string]string{"env-json": envJSON})
			if res.code != c.code {
				t.Fatalf("código %d, quiero %d\n%s", res.code, c.code, res.stdout)
			}
			if n := f.count(http.MethodPut); n != c.puts {
				t.Errorf("%d PUT, quiero %d", n, c.puts)
			}
			if n := f.count(http.MethodPost); n != c.posts {
				t.Errorf("%d POST, quiero %d", n, c.posts)
			}
			if c.errHas != "" {
				if e := res.lastError(t); !strings.Contains(e, c.errHas) {
					t.Errorf("error = %q, quiero que diga %q", e, c.errHas)
				}
			}
			if res.elapsed < c.minElapsed {
				t.Errorf("tardó %s, quiero al menos %s (el Retry-After)", res.elapsed, c.minElapsed)
			}
			if c.maxElapsed > 0 && res.elapsed > c.maxElapsed {
				t.Errorf("tardó %s, quiero a lo más %s", res.elapsed, c.maxElapsed)
			}
		})
	}
}

func TestPostNotRetried(t *testing.T) {
	t.Parallel()
	t.Run("503", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{errStep(http.StatusServiceUnavailable, "docker_unavailable", "Docker no responde")}})
		res := runAction(t, f.URL, nil)
		if res.code != 1 || f.count(http.MethodPost) != 1 {
			t.Errorf("código %d con %d POST, quiero 1 y 1", res.code, f.count(http.MethodPost))
		}
		if e := res.lastError(t); !strings.Contains(e, "docker_unavailable") || !strings.Contains(e, "Docker no responde") {
			t.Errorf("error = %q", e)
		}
	})
	t.Run("sin respuesta", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{{hijack: true}}})
		res := runAction(t, f.URL, nil)
		if res.code != 1 || f.count(http.MethodPost) != 1 {
			t.Errorf("código %d con %d POST, quiero 1 y 1", res.code, f.count(http.MethodPost))
		}
		if e := res.lastError(t); !strings.Contains(e, "revísalo en el panel") {
			t.Errorf("error = %q", e)
		}
		if _, ok := res.outputs["deploy-id"]; ok {
			t.Errorf("salidas = %v, sin deploy-id", res.outputs)
		}
	})
}

func TestPollRetriesTransient(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{log: []step{
		errStep(http.StatusServiceUnavailable, "docker_unavailable", "Docker no responde"),
		{code: http.StatusTooManyRequests},
		{hijack: true},
		logStep("", 0, "succeeded", nil),
	}})
	res := runAction(t, f.URL, nil)
	if res.code != 0 {
		t.Fatalf("código %d, quiero 0\n%s", res.code, res.stdout)
	}
	if n := f.count(http.MethodGet); n != 4 {
		t.Errorf("%d GET del log, quiero 4", n)
	}
}

func TestErrorsShowCodeAndMessage(t *testing.T) {
	t.Parallel()
	t.Run("no_route", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{errStep(http.StatusNotFound, "no_route", "la ruta no existe")}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "todavía no tiene la API de deploys") {
			t.Errorf("código %d, error %q", res.code, e)
		}
	})
	t.Run("out_of_scope", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{errStep(http.StatusForbidden, "out_of_scope", "el token no alcanza a ese proyecto")}})
		res := runAction(t, f.URL, nil)
		e := res.lastError(t)
		if res.code != 1 || !strings.Contains(e, "403") || !strings.Contains(e, "out_of_scope") || !strings.Contains(e, "el token no alcanza a ese proyecto") {
			t.Errorf("código %d, error %q", res.code, e)
		}
	})
	t.Run("sin JSON", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{{code: http.StatusBadGateway, body: "<html>bad gateway</html>"}}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "502") {
			t.Errorf("código %d, error %q", res.code, e)
		}
	})
}

func TestInputsValidated(t *testing.T) {
	t.Parallel()
	bad := []struct {
		name   string
		inputs map[string]string
	}{
		{"url http", map[string]string{"url": "http://muelle.example.com"}},
		{"url localhost.example.com", map[string]string{"url": "http://localhost.example.com"}},
		{"url 127.0.0.1.nip.io", map[string]string{"url": "http://127.0.0.1.nip.io"}},
		{"url con ruta", map[string]string{"url": "https://muelle.example.com/panel"}},
		{"url con usuario", map[string]string{"url": "https://u:p@muelle.example.com"}},
		{"url con query", map[string]string{"url": "https://muelle.example.com?x=1"}},
		{"url vacía", map[string]string{"url": ""}},
		{"app de dos partes", map[string]string{"app": "a/b"}},
		{"app de cuatro partes", map[string]string{"app": "a/b/c/d"}},
		{"app con mayúsculas", map[string]string{"app": "A/b/c"}},
		{"app con ..", map[string]string{"app": "a/../c"}},
		{"env-json array", map[string]string{"env-json": `[1]`}},
		{"env-json con un número", map[string]string{"env-json": `{"a":1}`}},
		{"images-json string", map[string]string{"images-json": `"x"`}},
		{"env-secret", map[string]string{"env-secret": "quizás"}},
		{"timeout 0", map[string]string{"timeout": "0"}},
		{"timeout abc", map[string]string{"timeout": "abc"}},
		{"timeout negativo", map[string]string{"timeout": "-5"}},
		{"token vacío", map[string]string{"token": ""}},
		{"token con espacio", map[string]string{"token": "mk_ con espacio"}},
		{"token con CRLF", map[string]string{"token": "mk_x\r\nX-Evil: 1"}},
		{"token no ASCII", map[string]string{"token": "mk_contraseña"}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newFake(t, routes{})
			res := runAction(t, f.URL, c.inputs)
			if res.code != 1 || len(res.errors()) == 0 {
				t.Errorf("código %d, errores %v; quiero 1 con ::error::\n%s%s", res.code, res.errors(), res.stdout, res.stderr)
			}
			if n := len(f.requests()); n != 0 {
				t.Errorf("%d peticiones, quiero 0: las entradas se validan antes", n)
			}
			if tok, ok := c.inputs["token"]; ok && tok != "" {
				for _, part := range []string{tok, "X-Evil", "contraseña", "con espacio"} {
					if strings.Contains(tok, part) && strings.Contains(res.stdout+res.stderr, part) {
						t.Errorf("la salida repite el token (%q):\n%s%s", part, res.stdout, res.stderr)
					}
				}
			}
			if strings.Contains(res.stdout, "::add-mask::") {
				t.Error("::add-mask:: antes de validar")
			}
		})
	}

	// Los que sí sirven: sin env-json, la primera petición es el POST, que no
	// conecta (puerto 1).
	for _, url := range []string{"https://[::1]:1/", "http://localhost:1", "http://127.0.0.1:1"} {
		t.Run("sirve "+url, func(t *testing.T) {
			t.Parallel()
			res := runAction(t, url, nil)
			if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "revísalo en el panel") {
				t.Errorf("código %d, error %q; quiero el de no respuesta del POST", res.code, e)
			}
		})
	}
}

func TestOutputsValidated(t *testing.T) {
	t.Parallel()
	t.Run("id", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{post: []step{jsonStep(http.StatusAccepted, map[string]any{
			"id": "145\nstatus=succeeded", "status": "queued", "status_url": statusURL,
		})}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "campo id ") {
			t.Errorf("código %d, error %q", res.code, e)
		}
		if regexp.MustCompile(`(?m)^(status|deploy-id)=`).MatchString(res.rawOutputs) {
			t.Errorf("GITHUB_OUTPUT = %q, sin status ni deploy-id", res.rawOutputs)
		}
	})
	t.Run("status del log", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{log: []step{jsonStep(http.StatusOK, map[string]any{
			"chunk": "", "next_offset": 0, "status": "succeeded\nx=y", "superseded_by": nil,
		})}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "campo status ") {
			t.Errorf("código %d, error %q", res.code, e)
		}
		if _, ok := res.outputs["x"]; ok || strings.Contains(res.rawOutputs, "status=") {
			t.Errorf("GITHUB_OUTPUT = %q", res.rawOutputs)
		}
	})
	t.Run("next_offset", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{log: []step{jsonStep(http.StatusOK, map[string]any{
			"chunk": "x\n", "next_offset": "1&offset=0", "status": "running", "superseded_by": nil,
		})}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "campo next_offset ") {
			t.Errorf("código %d, error %q", res.code, e)
		}
		for _, r := range f.requests() {
			if strings.Contains(r.uri, "&") {
				t.Errorf("pidió %s", r.uri)
			}
		}
		if n := f.count(http.MethodGet); n != 1 {
			t.Errorf("%d GET, quiero 1", n)
		}
	})
	t.Run("superseded_by", func(t *testing.T) {
		t.Parallel()
		f := newFake(t, routes{log: []step{jsonStep(http.StatusOK, map[string]any{
			"chunk": "", "next_offset": 0, "status": "superseded", "superseded_by": "146\n::error::x",
		})}})
		res := runAction(t, f.URL, nil)
		if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "campo superseded_by ") {
			t.Errorf("código %d, error %q", res.code, e)
		}
		if slices.Contains(strings.Split(res.stdout, "\n"), "::error::x") {
			t.Errorf("stdout trae el ::error:: de muelle:\n%s", res.stdout)
		}
	})
}

func TestEnvJSONLarge(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", 100<<10)
	b, _ := json.Marshal(map[string]string{"DB_CERT": big, "DB_URL": "postgres://db"})
	f := newFake(t, routes{})
	res := runAction(t, f.URL, map[string]string{"env-json": string(b)})
	if res.code != 0 {
		t.Fatalf("código %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	reqs := f.requests()
	if len(reqs) == 0 || reqs[0].method != http.MethodPut {
		t.Fatalf("peticiones = %d, quiero el PUT primero", len(reqs))
	}
	mustEqualJSON(t, reqs[0].body, map[string]any{"vars": map[string]string{"DB_CERT": big, "DB_URL": "postgres://db"}, "secret": true})
}

func TestEnvJSONErrorHidesContent(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{})
	res := runAction(t, f.URL, map[string]string{"env-json": `{"DB_URL": "postgres://u:clave-secreta@db`})
	if res.code != 1 || len(res.errors()) == 0 {
		t.Errorf("código %d, errores %v", res.code, res.errors())
	}
	if strings.Contains(res.stdout+res.stderr, "clave-secreta") {
		t.Errorf("la salida trae el contenido de env-json:\n%s%s", res.stdout, res.stderr)
	}
	if n := len(f.requests()); n != 0 {
		t.Errorf("%d peticiones, quiero 0", n)
	}
}

// scriptLines son las líneas de deploy.sh sin comentarios y con las líneas
// continuadas (\ al final) unidas.
func scriptLines(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("deploy.sh")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.ReplaceAll(string(raw), "\\\n", " ")
	var lines []string
	for l := range strings.SplitSeq(joined, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestSecretsStayOffArgv(t *testing.T) {
	lines := scriptLines(t)

	// env-json solo lo lee jq del entorno (env.INPUT_ENV_JSON, que solo jq
	// entiende): nunca $INPUT_ENV_JSON, que bash expandiría en un argumento de
	// otro comando o de --arg/--argjson.
	envJSON := 0
	for _, l := range lines {
		for i := strings.Index(l, "INPUT_ENV_JSON"); i >= 0; {
			envJSON++
			if !strings.HasSuffix(l[:i], "env.") {
				t.Errorf("INPUT_ENV_JSON fuera de env.INPUT_ENV_JSON de jq: %q", l)
			}
			next := strings.Index(l[i+1:], "INPUT_ENV_JSON")
			if next < 0 {
				break
			}
			i += 1 + next
		}
	}
	if envJSON == 0 {
		t.Error("deploy.sh no lee env.INPUT_ENV_JSON")
	}

	// El token: su validación, el archivo de headers y el ::add-mask::, todos
	// builtins de bash (sin exec).
	allowed := map[string]*regexp.Regexp{
		"validación":         regexp.MustCompile(`^if ! \[\[ \$\{INPUT_TOKEN:-\} =~ \$re \]\]; then$`),
		"archivo de headers": regexp.MustCompile(`^printf 'Authorization: Bearer %s\\n' "\$INPUT_TOKEN" >"\$tmp/auth"$`),
		"::add-mask::":       regexp.MustCompile(`^printf '::add-mask::%s\\n' "\$INPUT_TOKEN"$`),
	}
	seen := map[string]bool{}
	for _, l := range lines {
		if !strings.Contains(l, "INPUT_TOKEN") {
			continue
		}
		ok := false
		for name, re := range allowed {
			if re.MatchString(l) {
				seen[name], ok = true, true
			}
		}
		if !ok {
			t.Errorf("INPUT_TOKEN fuera de lo permitido: %q", l)
		}
	}
	for name := range allowed {
		if !seen[name] {
			t.Errorf("deploy.sh no tiene la línea del token: %s", name)
		}
	}

	curlCall := regexp.MustCompile(`(^|[^\w-])curl\s+[-"$]`) // una llamada, no "for tool in curl jq"
	curls := 0
	for _, l := range lines {
		if curlCall.MatchString(l) {
			curls++
			if !strings.Contains(l, "--globoff") {
				t.Errorf("curl sin --globoff: %q", l)
			}
		}
		if regexp.MustCompile(`\bset\s+(-[a-z]*x|-o\s+xtrace)`).MatchString(l) {
			t.Errorf("xtrace imprimiría el token: %q", l)
		}
	}
	if curls == 0 {
		t.Error("deploy.sh no llama a curl")
	}

	muelleVar := regexp.MustCompile(`\$\{?(MUELLE_[A-Z_]+)`)
	for _, l := range lines {
		for _, m := range muelleVar.FindAllStringSubmatch(l, -1) {
			if m[1] != "MUELLE_ACTION_TEST_POLL" && m[1] != "MUELLE_ACTION_TEST_RETRY_DELAYS" {
				t.Errorf("deploy.sh lee %s: un env: del job podría cambiar la action", m[1])
			}
		}
	}
}

func TestCurlGlobOff(t *testing.T) {
	t.Parallel()
	globURL := appPath + "/deploys/[1-2]"
	f := newFake(t, routes{post: []step{createdStep(globURL)}})
	res := runAction(t, f.URL, nil)
	if res.code != 0 {
		t.Fatalf("código %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	reqs := f.requests()
	if len(reqs) != 2 || reqs[1].uri != globURL+"/log?offset=0" {
		t.Errorf("peticiones = %v, quiero el POST y un GET de %s/log?offset=0", reqs, globURL)
	}
}

func TestStatusURLMustBeLocal(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"https://otro.example.com" + statusURL,
		"//otro.example.com" + statusURL,
		"/api/v2/projects/auditfy/stages/staging/apps/api/deploys/145",
		statusURL + " x",
	} {
		t.Run(u, func(t *testing.T) {
			t.Parallel()
			f := newFake(t, routes{post: []step{createdStep(u)}})
			res := runAction(t, f.URL, nil)
			if e := res.lastError(t); res.code != 1 || !strings.Contains(e, "campo status_url ") {
				t.Errorf("código %d, error %q", res.code, e)
			}
			if n := len(f.requests()); n != 1 {
				t.Errorf("%d peticiones, quiero solo el POST", n)
			}
		})
	}
}

func TestEscapesWorkflowCommands(t *testing.T) {
	t.Parallel()
	f := newFake(t, routes{
		log: []step{logStep("", 0, "failed", nil)},
		get: []step{deployStep("failed", "100% roto\nsegunda línea\r")},
	})
	res := runAction(t, f.URL, nil)
	if res.code != 1 {
		t.Fatalf("código %d\n%s", res.code, res.stdout)
	}
	if e := res.lastError(t); !strings.Contains(e, "100%25 roto%0Asegunda línea%0D") {
		t.Errorf("error = %q", e)
	}
}
