package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
	"zdx-ban/internal/inference"
)

type Engine struct {
	BaseURL, Model    string
	Client            *http.Client
	Retries, MaxBytes int
}

func New(url, model string, timeout time.Duration) *Engine {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = timeout
	return &Engine{BaseURL: strings.TrimRight(url, "/"), Model: model, Client: &http.Client{Transport: transport}, Retries: 2, MaxBytes: 2 << 20}
}

type request struct {
	Model, Prompt, System string
	Stream                bool
	Format                any            `json:"format,omitempty"`
	Options               map[string]any `json:"options,omitempty"`
}
type chunk struct {
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	Error           string `json:"error"`
}

func (o *Engine) Generate(ctx context.Context, r inference.Request) (inference.Result, error) {
	return o.generate(ctx, r, false, nil)
}
func (o *Engine) GenerateStructured(ctx context.Context, r inference.Request, dst any) (inference.Result, error) {
	return o.generate(ctx, r, true, dst)
}
func (o *Engine) generate(ctx context.Context, r inference.Request, structured bool, dst any) (inference.Result, error) {
	temp, seed, max := r.Constraints.Temperature, r.Constraints.Seed, r.Constraints.MaxTokens
	if r.Temperature != 0 {
		temp = r.Temperature
	}
	if r.Seed != nil {
		seed = r.Seed
	}
	if r.MaxTokens != 0 {
		max = r.MaxTokens
	}
	opts := map[string]any{"temperature": temp}
	if seed != nil {
		opts["seed"] = *seed
	}
	if max > 0 {
		opts["num_predict"] = max
	}
	if len(r.Constraints.Stop) > 0 {
		opts["stop"] = r.Constraints.Stop
	}
	q := request{Model: o.Model, Prompt: r.Prompt, System: r.System, Stream: true, Options: opts}
	if structured {
		schema := r.StructuredSchema
		if schema == nil {
			var schemaErr error
			schema, schemaErr = schemaFor(dst)
			if schemaErr != nil {
				return inference.Result{}, inference.NewFailure(inference.ProviderResponseError, schemaErr)
			}
		}
		q.Format = schema
	}
	b, err := json.Marshal(q)
	if err != nil {
		return inference.Result{}, inference.NewFailure(inference.ProviderResponseError, err)
	}
	var last error
	var lastResult inference.Result
	for attempt := 0; attempt <= o.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return inference.Result{}, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond):
			}
		}
		out, e := o.call(ctx, r.RequestID, b)
		lastResult = out
		if e == nil {
			if structured {
				cleaned := cleanJSON(out.Output)
				if e = inference.DecodeStrict([]byte(cleaned), dst); e != nil {
					return out, inference.NewFailure(inference.ModelOutputMalformed, e)
				}
				out.Structured = json.RawMessage(cleaned)
			}
			return out, nil
		}
		last = e
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			break
		}
	}
	return lastResult, last
}

func schemaFor(dst any) (map[string]any, error) {
	if dst == nil {
		return nil, errors.New("structured output destination is nil")
	}
	return schemaForType(reflect.TypeOf(dst))
}

func schemaForType(t reflect.Type) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] == "-" {
					continue
				}
				if parts[0] != "" {
					name = parts[0]
				}
			}
			property, err := schemaForType(field.Type)
			if err != nil {
				return nil, fmt.Errorf("structured field %s: %w", field.Name, err)
			}
			properties[name] = property
			required = append(required, name)
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}, nil
	case reflect.Slice, reflect.Array:
		items, err := schemaForType(t.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	default:
		return nil, fmt.Errorf("unsupported structured output type %s", t)
	}
}
func (o *Engine) call(ctx context.Context, id string, b []byte) (inference.Result, error) {
	start := time.Now()
	result := inference.Result{RequestID: id}
	result.Call.StartedAt = start.UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return inference.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	result.Call.RequestInitiatedAt = time.Now().UTC()
	resp, err := o.Client.Do(req)
	if err != nil {
		result.Call.Duration = time.Since(start)
		result.Call.FailedAt = time.Now().UTC()
		code := inference.ProviderConnectionError
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			code = inference.ProviderTimeout
			result.Call.TimedOutAt = result.Call.FailedAt
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			code = inference.ProviderCancelled
			result.Call.CancelledAt = result.Call.FailedAt
		}
		var ue *url.Error
		if errors.As(err, &ue) && ue.Timeout() {
			code = inference.ProviderTimeout
			result.Call.TimedOutAt = result.Call.FailedAt
		}
		return result, inference.NewFailure(code, err)
	}
	defer resp.Body.Close()
	result.Call.HeadersReceivedAt = time.Now().UTC()
	result.Call.StreamOpened = true
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		result.Call.FailedAt = time.Now().UTC()
		result.Call.Duration = time.Since(start)
		return result, inference.NewFailure(inference.ProviderResponseError, fmt.Errorf("%w: status %d: %s", inference.ErrUnavailable, resp.StatusCode, strings.TrimSpace(string(msg))))
	}
	var out strings.Builder
	sawDone := false
	scan := bufio.NewScanner(resp.Body)
	scan.Buffer(make([]byte, 64*1024), o.MaxBytes)
	for scan.Scan() {
		result.Call.Chunks++
		var c chunk
		if err = json.Unmarshal(scan.Bytes(), &c); err != nil {
			result.Call.FailedAt = time.Now().UTC()
			result.Call.Duration = time.Since(start)
			return result, inference.NewFailure(inference.ModelOutputMalformed, err)
		}
		if c.Error != "" {
			result.Call.FailedAt = time.Now().UTC()
			result.Call.Duration = time.Since(start)
			return result, inference.NewFailure(inference.ProviderResponseError, fmt.Errorf("%w: %s", inference.ErrUnavailable, c.Error))
		}
		if out.Len()+len(c.Response) > o.MaxBytes {
			return result, inference.NewFailure(inference.ModelOutputMalformed, errors.New("response limit exceeded"))
		}
		out.WriteString(c.Response)
		if c.Response != "" {
			if result.Call.FirstContentAt.IsZero() {
				result.Call.FirstContentAt = time.Now().UTC()
			}
			result.Call.StreamProgressed = true
			result.Output = out.String()
			result.Text = result.Output
		}
		if c.Done {
			sawDone = true
			result.StopReason = c.DoneReason
		}
		result.Usage.PromptTokens = c.PromptEvalCount
		result.Usage.CompletionTokens = c.EvalCount
	}
	if err = scan.Err(); err != nil {
		result.Call.FailedAt = time.Now().UTC()
		result.Call.Duration = time.Since(start)
		code := inference.ProviderResponseError
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			code = inference.ProviderTimeout
			result.Call.TimedOutAt = result.Call.FailedAt
		}
		if errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled {
			code = inference.ProviderCancelled
			result.Call.CancelledAt = result.Call.FailedAt
		}
		return result, inference.NewFailure(code, err)
	}
	if !sawDone {
		result.Call.FailedAt = time.Now().UTC()
		result.Call.Duration = time.Since(start)
		return result, inference.NewFailure(inference.ProviderResponseError, errors.New("incomplete stream: missing done marker"))
	}
	result.Output = out.String()
	result.Text = result.Output
	result.Usage.Latency = time.Since(start)
	result.PromptTokens = result.Usage.PromptTokens
	result.CompletionTokens = result.Usage.CompletionTokens
	result.Latency = result.Usage.Latency
	result.Call.CompletedAt = time.Now().UTC()
	result.Call.Duration = result.Usage.Latency
	result.Call.StreamCompleted = true
	return result, nil
}
func cleanJSON(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "```json"), "```"), "```"))
}
func (o *Engine) Capabilities(context.Context) inference.Capabilities {
	return inference.Capabilities{inference.TextGeneration: true, inference.StructuredGeneration: true, inference.SeededGeneration: true, inference.TokenUsage: true, inference.ModelInspection: true}
}
func (o *Engine) Health(ctx context.Context) (inference.Health, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	r, err := o.Client.Do(req)
	if err != nil {
		return inference.Health{Detail: err.Error()}, fmt.Errorf("%w: %v", inference.ErrUnavailable, err)
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return inference.Health{Detail: fmt.Sprintf("status %d", r.StatusCode)}, inference.ErrUnavailable
	}
	return inference.Health{Available: true}, nil
}
func (o *Engine) ModelState(ctx context.Context) (inference.ModelInfo, error) {
	if _, err := o.Health(ctx); err != nil {
		return inference.ModelInfo{}, err
	}
	return inference.ModelInfo{Provider: "ollama", Model: o.Model, IdentityConfidence: "DECLARED", IdentitySource: "ollama deployment name"}, nil
}
