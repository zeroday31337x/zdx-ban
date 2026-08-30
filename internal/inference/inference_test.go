package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/inference/ollama"
)

func TestOllamaStructuredRequestUsesExactJSONSchema(t *testing.T) {
	var request map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(w, `{"response":"{\"evidence\":[\"measured\"],\"passed\":true}","done":true}`)
	}))
	defer s.Close()
	o := ollama.New(s.URL, "m", time.Second)
	o.Retries = 0
	var got struct {
		Evidence []string `json:"evidence"`
		Passed   bool     `json:"passed"`
	}
	if _, err := o.GenerateStructured(context.Background(), inference.Request{Prompt: "x"}, &got); err != nil {
		t.Fatal(err)
	}
	format, ok := request["format"].(map[string]any)
	if !ok || format["type"] != "object" || format["additionalProperties"] != false {
		t.Fatalf("format is not an exact object schema: %#v", request["format"])
	}
	properties := format["properties"].(map[string]any)
	evidence := properties["evidence"].(map[string]any)
	if evidence["type"] != "array" || evidence["items"].(map[string]any)["type"] != "string" {
		t.Fatalf("evidence schema=%#v", evidence)
	}
}

func TestOllamaSatisfiesGenericEngineAndCapabilities(t *testing.T) {
	var _ inference.Engine = ollama.New("http://invalid", "bin/ban", time.Second)
	o := ollama.New("http://invalid", "bin/ban", time.Second)
	c := o.Capabilities(context.Background())
	if !c.Supports(inference.StructuredGeneration) {
		t.Fatal("structured capability missing")
	}
}
func TestOllamaErrorsMapGeneric(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 503) }))
	defer s.Close()
	o := ollama.New(s.URL, "bin/ban", time.Second)
	o.Retries = 0
	if _, e := o.Generate(context.Background(), inference.Request{Prompt: "x"}); !errors.Is(e, inference.ErrUnavailable) {
		t.Fatalf("unmapped error: %v", e)
	}
}
func TestOllamaFailureTaxonomyAndPartialTelemetry(t *testing.T) {
	t.Run("malformed complete response", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "not-json") }))
		defer s.Close()
		o := ollama.New(s.URL, "m", time.Second)
		o.Retries = 0
		_, err := o.Generate(context.Background(), inference.Request{Prompt: "x"})
		if inference.FailureCodeOf(err) != inference.ModelOutputMalformed {
			t.Fatalf("code=%s err=%v", inference.FailureCodeOf(err), err)
		}
	})
	t.Run("partial stream timeout", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"response":"partial","done":false}`)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-r.Context().Done()
		}))
		defer s.Close()
		o := ollama.New(s.URL, "m", time.Second)
		o.Retries = 0
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		got, err := o.Generate(ctx, inference.Request{Prompt: "x"})
		if inference.FailureCodeOf(err) != inference.ProviderTimeout {
			t.Fatalf("code=%s err=%v", inference.FailureCodeOf(err), err)
		}
		if !got.Call.StreamOpened || !got.Call.StreamProgressed || got.Call.StreamCompleted || got.Text != "partial" {
			t.Fatalf("telemetry=%+v text=%q", got.Call, got.Text)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		o := ollama.New("http://127.0.0.1:1", "m", time.Second)
		o.Retries = 0
		_, err := o.Generate(ctx, inference.Request{Prompt: "x"})
		if inference.FailureCodeOf(err) != inference.ProviderCancelled {
			t.Fatalf("code=%s err=%v", inference.FailureCodeOf(err), err)
		}
	})
}

func TestNativeUnavailableAndDifferential(t *testing.T) {
	n := inference.NativeZDXEngine{Reason: "not linked"}
	if _, e := n.Generate(context.Background(), inference.Request{}); !errors.Is(e, inference.ErrUnavailable) {
		t.Fatal(e)
	}
	d := inference.Compare(context.Background(), "a", n, "b", n, inference.Request{})
	if d.ErrorA == "" || d.ErrorB == "" {
		t.Fatal("differential errors not preserved")
	}
}
