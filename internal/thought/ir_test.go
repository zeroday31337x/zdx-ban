package thought

import (
	"context"
	"encoding/json"
	"testing"
)

func TestIRRoundTripAndOptionalFields(t *testing.T) {
	v := IR{Version: Version, ID: "t1", Goal: "g"}
	b, e := v.MarshalDeterministic()
	if e != nil {
		t.Fatal(e)
	}
	got, e := Parse(b)
	if e != nil || got.ID != "t1" {
		t.Fatalf("round trip: %+v %v", got, e)
	}
	b2, _ := v.MarshalDeterministic()
	if string(b) != string(b2) {
		t.Fatal("serialization is not deterministic")
	}
}
func TestIRVersionMalformedAndUnknown(t *testing.T) {
	for _, b := range [][]byte{[]byte(`{"version":"2","id":"x","goal":"g"}`), []byte(`{"version":"1"}`), []byte(`{"version":"1","id":"x","goal":"g","evil":true}`), []byte(`not json`)} {
		if _, e := Parse(b); e == nil {
			t.Fatalf("accepted %s", b)
		}
	}
}
func TestCompilerRejectsModelShellShape(t *testing.T) {
	c := CanonicalCompiler{}
	if _, _, e := c.Parse(context.Background(), []byte(`{"shell":"rm -rf /"}`)); e == nil {
		t.Fatal("accepted non-ThoughtIR executable data")
	}
	v, _ := c.Compile(context.Background(), CompileInput{Goal: "g"})
	r, e := c.Render(context.Background(), v)
	if e != nil || !json.Valid(r.Data) {
		t.Fatal("invalid render")
	}
}
