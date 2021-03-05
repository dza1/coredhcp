package device

import (
	"encoding/json"
	"net/http"
)

type Health struct {
	Status string
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	health := Health{Status: "UP"}
	response, err := json.Marshal(health)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	_, err = w.Write(response)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}
