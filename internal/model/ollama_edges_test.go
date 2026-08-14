package model

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serverFor(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
}
func TestOllamaMalformedAndIncomplete(t *testing.T) {
	for name, body := range map[string]string{"malformed": "not-json\n", "incomplete": "{\"response\":\"x\",\"done\":false}\n", "unexpected": "{\"unexpected\":true}\n"} {
		t.Run(name, func(t *testing.T) {
			s := serverFor(body)
			defer s.Close()
			o := NewOllama(s.URL, "m", time.Second)
			o.Retries = 0
			if _, err := o.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
}
func TestOllamaMissingTokensAndFragmentedTransport(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, _ := w.(http.Flusher)
		for _, part := range []string{"{\"response\":\"hel", "lo\",\"done\":false}\n{\"response\":\"!\",", "\"done\":true}\n"} {
			fmt.Fprint(w, part)
			if f != nil {
				f.Flush()
			}
		}
	}))
	defer s.Close()
	o := NewOllama(s.URL, "m", time.Second)
	r, err := o.Generate(context.Background(), GenerateRequest{Prompt: "x"})
	if err != nil || r.Text != "hello!" || r.PromptTokens != 0 || r.CompletionTokens != 0 {
		t.Fatalf("response=%+v err=%v", r, err)
	}
}
func TestOllamaTimeoutCancellationConnection(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	o := NewOllama(s.URL, "m", 20*time.Millisecond)
	o.Retries = 0
	if _, err := o.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil {
		t.Fatal("timeout accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := o.Generate(ctx, GenerateRequest{Prompt: "x"}); err == nil {
		t.Fatal("cancel accepted")
	}
	s.Close()
	bad := NewOllama("http://127.0.0.1:1", "m", 20*time.Millisecond)
	bad.Retries = 0
	if _, err := bad.Generate(context.Background(), GenerateRequest{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatalf("connection error=%v", err)
	}
}
func TestOllamaFinalStructuredDecodeFailsUnknown(t *testing.T) {
	s := serverFor("{\"response\":\"{\\\"value\\\":7,\\\"extra\\\":1}\",\"done\":true}\n")
	defer s.Close()
	o := NewOllama(s.URL, "m", time.Second)
	o.Retries = 0
	var dst struct {
		Value int `json:"value"`
	}
	if _, err := o.GenerateStructured(context.Background(), GenerateRequest{Prompt: "x"}, &dst); err == nil {
		t.Fatal("unknown structured field accepted")
	}
}
