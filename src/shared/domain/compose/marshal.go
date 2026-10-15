package compose

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Marshal arma el compose final: JSON indentado con las claves ordenadas, y
// todo "$" de un valor (no de una clave) como "$$", porque docker stack
// deploy vuelve a interpolar (§6.3). JSON es YAML 1.2: docker stack deploy lo
// lee igual, sin las ambigüedades de tipos de YAML (§6.2). No modifica f.
func Marshal(f *File) ([]byte, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(escapeDollars(v)); err != nil { // Encode termina con "\n"
		return nil, err
	}
	return buf.Bytes(), nil
}

// escapeDollars duplica cada $ de los strings que son valores. Las claves de
// los mapas no se tocan: docker stack deploy no las interpola.
func escapeDollars(v any) any {
	switch x := v.(type) {
	case string:
		return strings.ReplaceAll(x, "$", "$$")
	case map[string]any:
		for k, e := range x {
			x[k] = escapeDollars(e)
		}
	case []any:
		for i, e := range x {
			x[i] = escapeDollars(e)
		}
	}
	return v
}
