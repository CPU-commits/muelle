package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

// proxy es GET /api/v1/proxy (contrato §3.3).
func (h *handler) proxy(w http.ResponseWriter, r *http.Request) {
	out, err := h.uc.ProxyStatus.Execute(r.Context())
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, out)
}
