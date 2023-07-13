package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const eofError = "EOF"

var q = newQueue(maxConcurrent)

type promptRequest struct {
	Prompt string `json:"prompt"`
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	var p promptRequest
	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	done := r.Context().Done()
	id := r.Context().Value(contextSubKey).(string)

	if !q.wait(id, done) {
		http.Error(w, "Can only access LLM once at a time", http.StatusConflict)
		return
	}
	defer q.release(id)

	tokens := make(chan string)
	errCh := make(chan error)

	resp, err := prompt(p)
	if err != nil {
		http.Error(w, "Failed to prompt LLM", http.StatusInternalServerError)
		return
	}

	go generate(resp, tokens, r.Context().Done(), errCh)

	for {
		select {
		case token := <-tokens:
			fmt.Fprint(w, token)
			flusher.Flush()
		case err := <-errCh:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		case <-done:
			return
		}
	}
}

func generate(resp *http.Response, tokens chan<- string, done <-chan struct{}, errCh chan<- error) {
	defer close(tokens)
	defer close(errCh)

	for {
		select {
		case <-done:
			return
		case <-time.After(llmTimeout):
			errCh <- fmt.Errorf("LLM timed out after %v", llmTimeout)
			return
		default:
			data := make([]byte, 1024)
			_, err := resp.Body.Read(data)
			if err != nil {
				//lint:ignore ST1005 serving error to frontend
				errCh <- fmt.Errorf("Failed to retrieve token from LLM")
				return
			}
			select {
			case tokens <- string(data):
				break
			default:
				return
			}
		}
	}
}

func prompt(p promptRequest) (*http.Response, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", llmHost, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Connection", "keep-alive")

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil && err.Error() != eofError {
		return nil, err
	}

	return resp, nil
}
