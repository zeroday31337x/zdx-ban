package inference

import "context"

// NativeZDXEngine is an explicit extension point. It reports unavailable until
// a native ZDX inference runtime is supplied; it never silently falls back.
type NativeZDXEngine struct{ Reason string }

func (n NativeZDXEngine) Generate(context.Context, Request) (Result, error) {
	return Result{}, ErrUnavailable
}
func (n NativeZDXEngine) GenerateStructured(context.Context, Request, any) (Result, error) {
	return Result{}, ErrUnavailable
}
func (n NativeZDXEngine) Capabilities(context.Context) Capabilities { return Capabilities{} }
func (n NativeZDXEngine) Health(context.Context) (Health, error) {
	return Health{Detail: n.Reason}, ErrUnavailable
}
func (n NativeZDXEngine) ModelState(context.Context) (ModelInfo, error) {
	return ModelInfo{Provider: "native-zdx", IdentityConfidence: "UNAVAILABLE", IdentitySource: "extension-point"}, ErrUnavailable
}
