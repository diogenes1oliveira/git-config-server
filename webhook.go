package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// StartWebhookServer starts a simple http server to listen to POST requests.
//
// ctx is a context that can be used to stop the server.
//
// port is the port to bind the webhook to.
//
// onInvoked is a function to be called when a valid request is received.
func StartWebhookServer(ctx context.Context, port int, tokenHeader, tokenValue string, onInvoked func() error) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK
		defer func() {
			printLog(r, status)
		}()

		if r.Method == "GET" && strings.Contains(r.RequestURI, "/health") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}

		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			http.Error(w, "Invalid request method", status)
			return
		}

		if tokenHeader != "" {
			headerValue := r.Header.Get(tokenHeader)
			headerValue = strings.TrimSpace(headerValue)

			if headerValue != tokenValue {
				status = http.StatusForbidden
				http.Error(w, "Not authorized", status)
				return
			}
		}

		log.Printf("invoking webhook handler\n")
		err := onInvoked()
		if err != nil {
			log.Printf("webhook handler failed: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Printf("stopping webhook server")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("webhook server shutdown: %v", err)
		}
	}()

	errCh := make(chan error)

	go func() {
		defer close(errCh)

		log.Printf("starting webhook server on :%d", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("failed to listen on %d: %v", port, err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-time.After(1 * time.Second):
		return nil
	}
}

func printLog(r *http.Request, statusCode int) {
	remoteAddr := r.RemoteAddr
	if remoteAddr == "" {
		remoteAddr = "-"
	}

	method := r.Method
	if method == "" {
		method = "-"
	}

	uri := r.RequestURI
	if uri == "" {
		uri = "-"
	}

	protocol := r.Proto
	if protocol == "" {
		protocol = "-"
	}

	logEntry := fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d -",
		remoteAddr,
		time.Now().Format("02/Jan/2006:15:04:05 -0700"),
		method,
		uri,
		protocol,
		statusCode,
	)

	fmt.Fprintln(os.Stderr, logEntry)
}
