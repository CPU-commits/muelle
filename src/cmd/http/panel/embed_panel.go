//go:build panel

package panel

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

// build es web/build (pnpm build), copiado en build/ antes de compilar con
// -tags panel. Sin all: se perderían _app/ y los demás nombres que empiezan
// con _ o con . (web/CLAUDE.md, CSP).
//
//go:embed all:build
var build embed.FS

// Embedded devuelve el handler del panel embebido. Falla si el build no trae
// 200.html o csp-header.txt, o si la CSP no sirve: muelle no arranca.
func Embedded() (http.Handler, error) {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		return nil, fmt.Errorf("panel: %w", err)
	}
	h, err := New(sub)
	if err != nil {
		return nil, fmt.Errorf("panel embebido: %w", err)
	}
	return h, nil
}
