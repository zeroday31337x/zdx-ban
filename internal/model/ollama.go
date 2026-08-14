package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Ollama struct {
	BaseURL, Model    string
	Client            *http.Client
	Retries, MaxBytes int
}

func NewOllama(url, model string, timeout time.Duration) *Ollama {
	return &Ollama{strings.TrimRight(url, "/"), model, &http.Client{Timeout: timeout}, 2, 2 << 20}
}

type ollamaRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	System  string         `json:"system,omitempty"`
	Stream  bool           `json:"stream"`
	Format  string         `json:"format,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}
type ollamaChunk struct {
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error"`
}

func (o *Ollama) Generate(ctx context.Context, r GenerateRequest) (GenerateResponse, error) {
	return o.generate(ctx, r, false, nil)
}
func (o *Ollama) GenerateStructured(ctx context.Context, r GenerateRequest, dst any) (GenerateResponse, error) {
	return o.generate(ctx, r, true, dst)
}
func (o *Ollama) generate(ctx context.Context, r GenerateRequest, structured bool, dst any) (GenerateResponse, error) {
	opts := map[string]any{"temperature": r.Temperature}
	if r.Seed != nil {
		opts["seed"] = *r.Seed
	}
	if r.MaxTokens > 0 {
		opts["num_predict"] = r.MaxTokens
	}
	q := ollamaRequest{Model: o.Model, Prompt: r.Prompt, System: r.System, Stream: true, Options: opts}
	if structured {
		q.Format = "json"
	}
	b, err := json.Marshal(q)
	if err != nil {
		return GenerateResponse{}, err
	}
	var last error
	for attempt := 0; attempt <= o.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return GenerateResponse{}, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond):
			}
		}
		resp, e := o.call(ctx, b)
		if e == nil {
			if structured {
				if e = decodeModelJSON(resp.Text, dst); e != nil {
					return resp, fmt.Errorf("invalid structured output: %w", e)
				}
			}
			return resp, nil
		}
		last = e
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			break
		}
	}
	return GenerateResponse{}, last
}
func (o *Ollama) call(ctx context.Context, b []byte) (GenerateResponse, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return GenerateResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := o.Client.Do(req)
	if err != nil {
		return GenerateResponse{}, err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		return GenerateResponse{}, fmt.Errorf("ollama status %d: %s", r.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out strings.Builder
	result := GenerateResponse{}
	sawDone := false
	scan := bufio.NewScanner(r.Body)
	scan.Buffer(make([]byte, 64*1024), o.MaxBytes)
	for scan.Scan() {
		var c ollamaChunk
		if err = json.Unmarshal(scan.Bytes(), &c); err != nil {
			return result, err
		}
		if c.Error != "" {
			return result, errors.New(c.Error)
		}
		if out.Len()+len(c.Response) > o.MaxBytes {
			return result, errors.New("response limit exceeded")
		}
		out.WriteString(c.Response)
		if c.Done {
			sawDone = true
		}
		result.PromptTokens = c.PromptEvalCount
		result.CompletionTokens = c.EvalCount
	}
	if err = scan.Err(); err != nil {
		return result, err
	}
	if !sawDone {
		return result, errors.New("incomplete ollama stream: missing done marker")
	}
	result.Text = out.String()
	result.Latency = time.Since(start)
	return result, nil
}
func decodeModelJSON(s string, dst any) error {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "```json"), "```"), "```"))
	return DecodeStrict([]byte(s), dst)
}
func (o *Ollama) Health(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	r, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return fmt.Errorf("health status %d", r.StatusCode)
	}
	return nil
}
func (o *Ollama) ModelInfo(ctx context.Context) (Info, error) {
	if err := o.Health(ctx); err != nil {
		return Info{}, err
	}
	return Info{Provider: "ollama", Model: o.Model}, nil
}
