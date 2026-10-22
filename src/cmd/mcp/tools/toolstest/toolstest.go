// Package toolstest conecta un cliente del SDK a los servidores de un
// tools.Kit por el transporte en memoria, con el Principal en el ctx, para
// probar las herramientas sin HTTP. Los tests de los módulos lo usan sin
// importar el SDK.
package toolstest

import (
	"context"
	jsonv2 "encoding/json/v2"
	"slices"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Session es un cliente del SDK conectado a un servidor del Kit.
type Session struct {
	t  testing.TB
	cs *sdk.ClientSession
}

// Connect conecta un cliente al servidor que le toca a p (Kit.Server) con p
// en el ctx, como lo deja el handler HTTP después de autenticar.
func Connect(t testing.TB, k *tools.Kit, p entity.Principal) *Session {
	t.Helper()
	return ConnectServer(t, k.Server(p), p)
}

// ConnectServer conecta un cliente a s con p en el ctx, aunque no sea el
// servidor que le toca (para probar la defensa en profundidad del kit).
func ConnectServer(t testing.TB, s *sdk.Server, p entity.Principal) *Session {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	ss, err := s.Connect(entity.WithPrincipal(ctx, p), serverTransport, nil)
	if err != nil {
		t.Fatalf("toolstest: conectando el servidor: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "toolstest", Version: "test"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		ss.Close()
		t.Fatalf("toolstest: conectando el cliente: %v", err)
	}
	t.Cleanup(func() {
		cs.Close()
		ss.Close()
	})
	return &Session{t: t, cs: cs}
}

// Client es la sesión del SDK, para lo que Session no cubre.
func (s *Session) Client() *sdk.ClientSession { return s.cs }

// Call llama a la herramienta name con args (nil: sin argumentos) y devuelve
// el text del único bloque de la respuesta y si es un error de ejecución.
// Falla el test si la llamada da un error de protocolo, o si la respuesta
// no es un solo TextContent sin structuredContent (contrato §8.3).
func (s *Session) Call(name string, args any) (text string, isError bool) {
	s.t.Helper()
	res, err := s.CallResult(name, args)
	if err != nil {
		s.t.Fatalf("toolstest: %s: %v", name, err)
	}
	if res.StructuredContent != nil {
		s.t.Fatalf("toolstest: %s: la respuesta trae structuredContent: %v", name, res.StructuredContent)
	}
	if len(res.Content) != 1 {
		s.t.Fatalf("toolstest: %s: la respuesta trae %d bloques, esperaba 1", name, len(res.Content))
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		s.t.Fatalf("toolstest: %s: el bloque es %T, esperaba texto", name, res.Content[0])
	}
	return tc.Text, res.IsError
}

// CallResult es Call sin revisar nada: el resultado del SDK, o su error de
// protocolo (una herramienta que el servidor no tiene).
func (s *Session) CallResult(name string, args any) (*sdk.CallToolResult, error) {
	return s.CallContext(context.Background(), name, args)
}

// CallContext es CallResult con ctx: cancelarlo cancela la llamada en el
// servidor.
func (s *Session) CallContext(ctx context.Context, name string, args any) (*sdk.CallToolResult, error) {
	return s.cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
}

// OK es Call de una llamada que tiene que salir bien: falla el test si da
// error, y devuelve la salida decodificada en T.
func OK[T any](s *Session, name string, args any) T {
	s.t.Helper()
	text, isErr := s.Call(name, args)
	if isErr {
		s.t.Fatalf("toolstest: %s: error: %s", name, text)
	}
	return Decode[T](s.t, text)
}

// Fail es Call de una llamada que tiene que fallar con el JSON de error de la
// API: devuelve su code y su message.
func Fail(s *Session, name string, args any) (code, message string) {
	s.t.Helper()
	text, isErr := s.Call(name, args)
	if !isErr {
		s.t.Fatalf("toolstest: %s: esperaba un error, salió %s", name, text)
	}
	e := Decode[struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}](s.t, text)
	if e.Error.Code == "" {
		s.t.Fatalf("toolstest: %s: el error no tiene code: %s", name, text)
	}
	return e.Error.Code, e.Error.Message
}

// Tools lista las herramientas del servidor.
func (s *Session) Tools() []*sdk.Tool {
	s.t.Helper()
	res, err := s.cs.ListTools(context.Background(), nil)
	if err != nil {
		s.t.Fatalf("toolstest: tools/list: %v", err)
	}
	return res.Tools
}

// ToolNames son los nombres de Tools, ordenados.
func (s *Session) ToolNames() []string {
	s.t.Helper()
	var names []string
	for _, t := range s.Tools() {
		names = append(names, t.Name)
	}
	slices.Sort(names)
	return names
}

// Decode decodifica text en T (json/v2, estricto con la sintaxis).
func Decode[T any](t testing.TB, text string) T {
	t.Helper()
	var v T
	if err := jsonv2.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("toolstest: decodificando %s: %v", text, err)
	}
	return v
}
