package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func productsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Request reached backend",
		"path":    r.URL.Path,
	})
}

func cartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "Cart request reached backend",
		"path":    r.URL.Path,
	})
}

func main() {
	http.HandleFunc("/api/products", productsHandler)
	http.HandleFunc("/api/cart", cartHandler)

	log.Println("Backend running on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}