// Package tools es el kit con que los módulos registran sus herramientas MCP
// (contrato §8.3, diseño del MCP §12): arma los dos servidores del SDK (el
// completo y el de solo lectura), los esquemas de entrada con las reglas de
// los slugs y los rangos, las anotaciones, y envuelve cada llamada con lo
// mismo que hace utils.Kit en REST: el alcance del token (Principal.Allows)
// antes de leer nada, los errores con el code y el message de REST, y además
// el límite de llamadas a la vez por token, el plazo y el tope del
// resultado.
//
// Solo src/cmd/mcp importa el SDK: los módulos registran con Register y no
// lo ven.
package tools

import (
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const (
	// DefaultTimeout es el plazo de una llamada (contrato §8.5).
	DefaultTimeout = 20 * time.Second
	// MaxCallsPerToken son las llamadas a la vez de un mismo token; la que
	// sigue recibe rate_limited sin correr (contrato §8.5).
	MaxCallsPerToken = 8
	// MaxDescription es el largo, en caracteres, desde el que Claude Code
	// corta la descripción de una herramienta y las instrucciones del
	// servidor.
	MaxDescription = 4096
	// RequiresUserInteraction es la clave de _meta con que Claude Code pide
	// confirmación en cada llamada, en cualquier modo de permisos (contrato §8.3).
	RequiresUserInteraction = "anthropic/requiresUserInteraction"
)

// Kind dice si una herramienta lee o escribe.
type Kind int

const (
	Read  Kind = iota // en los dos servidores
	Write             // solo en el completo; con aprobación de una persona en cada llamada
)

// Tool describe una herramienta.
type Tool struct {
	Name, Title, Description string
	Kind                     Kind
	Idempotent               bool             // solo Write: Read siempre lo es
	Untrusted                bool             // la salida trae texto de las apps: suma "warning"
	Bounds                   map[string]Range // minimum y maximum de cada entero de la entrada
	Defaults                 map[string]any   // el "default" del esquema; lo aplica el SDK
	Timeout                  time.Duration    // 0: DefaultTimeout
}

// Range es el rango de un entero de la entrada. Max 0 es sin maximum (id,
// offset).
type Range struct{ Min, Max int }

// Scope es el alcance que pide una llamada: Principal.Allows(Project, Stage,
// write), con write en las herramientas de escritura.
type Scope struct{ Project, Stage string }

// Timeouter lo implementa la entrada de una herramienta cuyo plazo depende de
// sus argumentos (get_deploy con wait_s). Un CallTimeout mayor que 0
// reemplaza a Tool.Timeout.
type Timeouter interface{ CallTimeout() time.Duration }

// Config arma el Kit.
type Config struct {
	// Classify da el code y el message de REST para un error (utils.Kit.Classify).
	Classify func(err error) (code, message string, ok bool)
	Log      *slog.Logger
	// Version es la de muelle (muelle version, "dev" en desarrollo).
	Version string
	// Instructions son las instrucciones del servidor (contrato §8.2).
	Instructions string
}

// Kit tiene los dos servidores del SDK y lo que envuelve cada llamada.
type Kit struct {
	full, readOnly *sdk.Server
	classify       func(error) (string, string, bool)
	log            *slog.Logger

	mu    sync.Mutex
	calls map[string]int // llamadas en curso por token; la entrada se borra en 0
	names map[string]bool
}

// NewKit arma los dos servidores, sin herramientas. Hace panic sin
// Config.Classify o con instrucciones de MaxDescription caracteres o más.
func NewKit(c Config) *Kit {
	if c.Classify == nil {
		panic("tools: NewKit sin Config.Classify")
	}
	if utf8.RuneCountInString(c.Instructions) >= MaxDescription {
		panic(fmt.Sprintf("tools: las instrucciones tienen %d caracteres; Claude Code corta a los %d", utf8.RuneCountInString(c.Instructions), MaxDescription))
	}
	log := c.Log
	if log == nil {
		log = slog.Default()
	}
	version := c.Version
	if version == "" {
		version = "dev"
	}
	impl := &sdk.Implementation{Name: "muelle", Title: "muelle", Version: version}
	options := func() *sdk.ServerOptions {
		// Capabilities fija {"tools":{}}: sin esto el SDK anuncia logging y
		// tools.listChanged, y muelle no tiene ninguno de los dos.
		// SetCacheable deja tools/list y server/discover en private y ttlMs
		// 0: la lista depende del token (11 herramientas o 15), y el
		// default del SDK, public, dejaría que un intermediario la
		// compartiera entre tokens.
		return &sdk.ServerOptions{
			Instructions: c.Instructions,
			Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
			SetCacheable: func(_ context.Context, _ sdk.Request, c *sdk.Cacheable) {
				c.TTLMs, c.CacheScope = 0, "private"
			},
		}
	}
	return &Kit{
		full: sdk.NewServer(impl, options()), readOnly: sdk.NewServer(impl, options()),
		classify: c.Classify, log: log, calls: map[string]int{}, names: map[string]bool{},
	}
}

// Server es el servidor que le toca a p: el de solo lectura, sin las
// herramientas de escritura, para un token de solo lectura; el completo
// para los demás.
func (k *Kit) Server(p entity.Principal) *sdk.Server {
	if p.ReadOnly() {
		return k.readOnly
	}
	return k.full
}

// Full es el servidor con todas las herramientas.
func (k *Kit) Full() *sdk.Server { return k.full }

// ReadOnly es el servidor con solo las herramientas de lectura.
func (k *Kit) ReadOnly() *sdk.Server { return k.readOnly }

// Register registra una herramienta: las de lectura en los dos servidores,
// las de escritura solo en el completo. scope da el alcance de una llamada
// con sus argumentos ya validados por el esquema; nil es una herramienta que
// filtra por el Principal (solo list_projects). h devuelve la salida, que el
// kit serializa (Encode), o un error, que el kit clasifica.
//
// Hace panic (un error de programación, como un patrón inválido del mux) si
// el nombre se repite, si el título o la descripción faltan o la
// descripción pasa MaxDescription, o si el esquema no se puede armar (ver
// inputSchema).
func Register[In any](k *Kit, t Tool, scope func(In) *Scope, h func(ctx context.Context, in In) (any, error)) {
	switch {
	case t.Name == "" || t.Title == "" || t.Description == "":
		panic(fmt.Sprintf("tools: la herramienta %q no tiene nombre, título o descripción", t.Name))
	case utf8.RuneCountInString(t.Description) >= MaxDescription:
		panic(fmt.Sprintf("tools: la descripción de %q tiene %d caracteres; Claude Code corta a los %d", t.Name, utf8.RuneCountInString(t.Description), MaxDescription))
	}
	k.mu.Lock()
	dup := k.names[t.Name]
	k.names[t.Name] = true
	k.mu.Unlock()
	if dup {
		panic(fmt.Sprintf("tools: la herramienta %q ya está registrada", t.Name))
	}
	schema, err := inputSchema[In](t)
	if err != nil {
		panic(fmt.Sprintf("tools: el esquema de %q: %v", t.Name, err))
	}
	st := &sdk.Tool{
		Name: t.Name, Title: t.Title, Description: t.Description,
		InputSchema: schema, Annotations: annotations(t),
	}
	if t.Kind == Write {
		st.Meta = sdk.Meta{RequiresUserInteraction: true}
	}
	handler := func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		var sc func() (*Scope, bool)
		if scope != nil {
			sc = func() (*Scope, bool) { s := scope(in); return s, s != nil }
		}
		timeout := t.Timeout
		if tt, ok := any(in).(Timeouter); ok && tt.CallTimeout() > 0 {
			timeout = tt.CallTimeout()
		}
		run := func(ctx context.Context) (any, error) { return h(ctx, in) }
		return k.call(ctx, t, sc, run, timeout), nil, nil
	}
	sdk.AddTool(k.full, st, handler)
	if t.Kind == Read {
		sdk.AddTool(k.readOnly, st, handler)
	}
}

// annotations son las de §8.3 del contrato: destructiveHint y openWorldHint
// van explícitos (el default de destructiveHint es true).
func annotations(t Tool) *sdk.ToolAnnotations {
	read := t.Kind == Read
	return &sdk.ToolAnnotations{
		Title:           t.Title,
		ReadOnlyHint:    read,
		DestructiveHint: new(!read),
		IdempotentHint:  read || t.Idempotent,
		OpenWorldHint:   new(false),
	}
}

// call envuelve una llamada, en el orden de §3.2 (paso 12) del diseño: via,
// read_only_token, el alcance, el límite por token, el plazo, la llamada, el
// error y el tope del resultado.
//
// Un panic en el alcance, la herramienta, el caso de uso o la serialización
// es internal: el SDK corre cada pedido en una goroutine suya, sin recover,
// y sin esto un panic tiraría el proceso entero (la API, el panel, los
// workers y el cron), no solo la llamada, como pasa en REST con net/http.
// Los defer de la llamada (su lugar en el límite, el plazo, los streams del
// caso de uso) corren antes de recuperarlo.
func (k *Kit) call(ctx context.Context, t Tool, scope func() (*Scope, bool), run func(context.Context) (any, error), timeout time.Duration) (res *sdk.CallToolResult) {
	p, ok := entity.PrincipalFrom(ctx)
	if !ok {
		k.log.Error("mcp: una llamada sin Principal", "tool", t.Name)
		return errorResult("internal", "error interno")
	}
	defer func() {
		if v := recover(); v != nil {
			// Sin los argumentos, como en fail: set_env trae valores.
			k.log.Error("mcp: error interno", "tool", t.Name, "token_id", tokenID(p),
				"panic", fmt.Sprint(v), "stack", string(debug.Stack()))
			res = errorResult("internal", "error interno")
		}
	}()
	ctx = entity.WithVia(ctx, entity.ViaMCP)
	write := t.Kind == Write
	if write && p.ReadOnly() {
		return errorResult("read_only_token", "el token es de solo lectura")
	}
	if scope != nil {
		s, ok := scope()
		if !ok {
			k.log.Error("mcp: la herramienta no dio su alcance", "tool", t.Name, "token_id", tokenID(p))
			return errorResult("internal", "error interno")
		}
		if !p.Allows(s.Project, s.Stage, write) {
			return errorResult("out_of_scope", "el token no tiene acceso a este recurso")
		}
	}
	key := callKey(p)
	if !k.acquire(key) {
		return errorResult("rate_limited", "demasiadas llamadas a la vez con este token; espera unos segundos")
	}
	defer k.release(key)

	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := run(callCtx)
	if err != nil {
		return k.fail(ctx, t, p, err)
	}
	text, err := Encode(out, t.Untrusted)
	switch {
	case err != nil:
		k.log.Error("mcp: error interno", "tool", t.Name, "token_id", tokenID(p), "err", err)
		return errorResult("internal", "error interno")
	case len(text) > MaxResultBytes:
		// Un bug: cada herramienta recorta antes (contrato §8.3).
		k.log.Error("mcp: error interno", "tool", t.Name, "token_id", tokenID(p),
			"err", fmt.Sprintf("el resultado mide %d bytes, más que los %d del tope", len(text), MaxResultBytes))
		return errorResult("internal", "error interno")
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(text)}}}
}

// fail responde un error con el code y el message de REST, o internal. El
// log nunca lleva los argumentos (set_env trae valores). Un context.Canceled
// con el ctx de la llamada ya cancelado es un cliente que se fue: va en Debug,
// como en utils.Kit.Fail.
func (k *Kit) fail(ctx context.Context, t Tool, p entity.Principal, err error) *sdk.CallToolResult {
	if code, msg, ok := k.classify(err); ok {
		return errorResult(code, msg)
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		k.log.Debug("mcp: el cliente se fue", "tool", t.Name, "token_id", tokenID(p))
	} else {
		k.log.Error("mcp: error interno", "tool", t.Name, "token_id", tokenID(p), "err", err)
	}
	return errorResult("internal", "error interno")
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorResult es un error de ejecución (isError) con el JSON de error de la
// API (contrato §8.4).
func errorResult(code, message string) *sdk.CallToolResult {
	b, err := jsonv2.Marshal(errorBody{Error: errorDetail{Code: code, Message: message}}, encodeOptions)
	if err != nil {
		b = []byte(`{"error":{"code":"internal","message":"error interno"}}`)
	}
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}}
}

// callKey identifica al principal en el límite de llamadas: el token, o la
// sesión (solo en los tests: el endpoint acepta solo tokens).
func callKey(p entity.Principal) string {
	if p.Token != nil {
		return "t:" + strconv.FormatInt(p.Token.ID, 10)
	}
	return "s:" + p.SessionHash
}

func tokenID(p entity.Principal) int64 {
	if p.Token != nil {
		return p.Token.ID
	}
	return 0
}

func (k *Kit) acquire(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.calls[key] >= MaxCallsPerToken {
		return false
	}
	k.calls[key]++
	return true
}

func (k *Kit) release(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.calls[key]--; k.calls[key] <= 0 {
		delete(k.calls, key)
	}
}

// rawDefault serializa el default de un campo para el esquema.
func rawDefault(v any) (json.RawMessage, error) {
	b, err := jsonv2.Marshal(v, encodeOptions)
	return json.RawMessage(b), err
}
