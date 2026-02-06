package console

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Hint     string          `json:"hint,omitempty"`
	File     string          `json:"file,omitempty"`
	Location *SourceLocation `json:"location,omitempty"`
}

type SourceLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

func WriteAPIError(w http.ResponseWriter, status int, err APIError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(err)
}
