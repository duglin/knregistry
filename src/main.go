package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	text := "Hello World!"
	rev := os.Getenv("K_REVISION") // K_REVISION=helloworld-7vh75
	msg := fmt.Sprintf("%s: %s\n", rev, text)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Got request\n")
		time.Sleep(500 * time.Millisecond)
		fmt.Fprint(w, msg)
	})
	fmt.Printf("Listening on port 8080 (rev: %s)\n", rev)
	http.ListenAndServe(":8080", nil)
}
