// Package httpx berisi middleware dan helper HTTP bersama.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// MaxBodyBytes adalah batas ukuran badan permintaan JSON.
const MaxBodyBytes = 1 << 20

// ErrorBody adalah bentuk standar respons galat.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// WriteJSON menulis v sebagai JSON dengan status yang diberikan.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("tulis respons JSON", "err", err)
	}
}

// WriteError menulis respons galat standar.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: code, Message: message})
}

// DecodeJSON membaca badan JSON ke dst. Field tak dikenal dan badan ganda ditolak.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode JSON: badan berisi lebih dari satu nilai")
	}
	return nil
}
