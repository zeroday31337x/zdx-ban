package model

import (
	"bytes"
	"context"
	"encoding/json"
)

type Provider interface {
	Generate(context.Context, GenerateRequest) (GenerateResponse, error)
	GenerateStructured(context.Context, GenerateRequest, any) (GenerateResponse, error)
	Health(context.Context) error
	ModelInfo(context.Context) (Info, error)
}

func DecodeStrict(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
