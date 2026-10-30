package httpx

import (
	"bytes"
	"encoding/json"
)

// Optional membedakan field JSON yang tidak dikirim, dikirim null, dan dikirim
// dengan nilai. Dipakai untuk PATCH, mis. menghapus tanggal dengan null.
type Optional[T any] struct {
	Set   bool // field ada di JSON
	Null  bool // field bernilai null
	Value T
}

// UnmarshalJSON hanya dipanggil bila field ada di JSON.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
