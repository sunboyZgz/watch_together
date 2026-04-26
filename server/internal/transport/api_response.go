package transport

import (
	"encoding/json"
	"net/http"
)

type apiResponse struct {
	Data any     `json:"data,omitempty"`
	Meta apiMeta `json:"meta"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
	Meta  apiMeta  `json:"meta"`
}

type apiMeta struct {
	RequestID string   `json:"requestId"`
	Page      *apiPage `json:"page,omitempty"`
}

type apiPage struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"nextCursor"`
}

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func writeAPISuccess(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Data: data,
		Meta: apiMeta{RequestID: "local"},
	})
}

func writeAPIError(w http.ResponseWriter, statusCode int, code string, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(apiErrorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: apiMeta{RequestID: "local"},
	})
}
