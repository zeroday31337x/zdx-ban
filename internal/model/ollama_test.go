package model

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaStreamingStructured(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			fmt.Fprint(w, `{"models":[]}`)
			return
		}
		fmt.Fprintln(w, `{"response":"{\"value\":","done":false}`)
		fmt.Fprintln(w, `{"response":"7}","done":true,"prompt_eval_count":2,"eval_count":3}`)
	}))
	defer s.Close()
	o := NewOllama(s.URL, "test", time.Second)
	var got struct {
		Value int `json:"value"`
	}
	r, err := o.GenerateStructured(context.Background(), GenerateRequest{Prompt: "x"}, &got)
	if err != nil || got.Value != 7 || r.CompletionTokens != 3 {
		t.Fatalf("got=%+v resp=%+v err=%v", got, r, err)
	}
	if err = o.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
}
