package model

type HealthResponse struct {
	Status string `json:"status"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type OKResponse struct {
	OK bool `json:"ok"`
}
