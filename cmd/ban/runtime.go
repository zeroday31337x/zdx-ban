package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"zdx-ban/internal/appconfig"
	"zdx-ban/internal/cognitive"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/modelstate"
	"zdx-ban/internal/thought"
	"zdx-ban/internal/training"
	"zdx-ban/internal/vm"
)

func runtimeCommand(mode string, args []string, config appconfig.Config) error {
	switch mode {
	case "model":
		return modelCommand(args, config.Model.FoundationID, config.Model.Name)
	case "thought":
		if len(args) < 2 || args[0] != "compile" {
			return fmt.Errorf("thought requires compile <goal>")
		}
		v, e := (thought.CanonicalCompiler{}).Compile(context.Background(), thought.CompileInput{Goal: strings.Join(args[1:], " ")})
		if e != nil {
			return e
		}
		return printJSON(v)
	case "vm":
		if len(args) == 0 || args[0] != "smoke" {
			return fmt.Errorf("vm requires smoke")
		}
		f := vm.FakeExecutor{Allowed: map[string]bool{"SET_STATE": true}}
		r, e := f.Execute(context.Background(), vm.Request{ExecutionID: "vm-smoke", InitialState: json.RawMessage(`{"value":0}`), Actions: []vm.Action{{Operation: "SET_STATE", Arguments: json.RawMessage(`{"value":1}`)}}, Bounds: vm.Bounds{MaxActions: 1, MaxOutputBytes: 1024}})
		if e != nil {
			return e
		}
		return printJSON(r)
	case "runtime":
		return runtimeSubcommand(args, config)
	case "training":
		return trainingCommand(args)
	default:
		return fmt.Errorf("unknown runtime command")
	}
}
func modelCommand(args []string, foundationID, modelName string) error {
	if len(args) == 0 {
		return fmt.Errorf("model requires inspect or fingerprint")
	}
	m := modelstate.DeclaredW0(foundationID, modelName)
	switch args[0] {
	case "inspect":
		return printJSON(m)
	case "fingerprint":
		fs := flag.NewFlagSet("model fingerprint", flag.ContinueOnError)
		artifact := fs.String("artifact", "", "model artifact to hash")
		config := fs.String("config", "Modelfile", "tokenizer/config artifact to hash")
		if e := fs.Parse(args[1:]); e != nil {
			return e
		}
		if *artifact != "" {
			h, e := hashFile(*artifact)
			if e != nil {
				return e
			}
			m.Foundation.ArtifactHash = h
			m.Foundation.Hash = &h
			m.Foundation.IdentityConfidence = "ARTIFACT_HASHED"
			m.Foundation.IdentitySource = *artifact
		}
		if *config != "" {
			h, e := hashFile(*config)
			if e != nil {
				return e
			}
			m.Foundation.TokenizerConfigHash = h
		}
		return printJSON(m)
	default:
		return fmt.Errorf("unknown model command %q", args[0])
	}
}
func runtimeSubcommand(args []string, config appconfig.Config) error {
	if len(args) == 0 {
		return fmt.Errorf("runtime requires capabilities or smoke")
	}
	o := newOllama(config)
	switch args[0] {
	case "capabilities":
		native := inference.NativeZDXEngine{Reason: "native ZDX runtime not linked in this checkout"}
		return printJSON(map[string]any{"ollama_harness": o.Capabilities(context.Background()), "native_zdx": native.Capabilities(context.Background()), "native_zdx_available": false, "thought_compiler": (thought.CanonicalCompiler{}).Capabilities(), "vm": (vm.FakeExecutor{}).Capabilities()})
	case "smoke":
		if len(args) < 2 {
			return fmt.Errorf("runtime smoke requires a goal")
		}
		ms := memory.NewMemoryStore()
		ts := &training.MemoryStore{}
		seed := 0
		l := cognitive.Loop{Compiler: thought.CanonicalCompiler{}, Inference: o, VM: vm.FakeExecutor{Allowed: map[string]bool{"SET_STATE": true}}, Memory: ms, Candidates: ts}
		r, e := l.Run(context.Background(), cognitive.Request{RunID: "cli-smoke", NodeID: "prediction-1", Goal: strings.Join(args[1:], " "), Input: strings.Join(args[1:], " "), InitialState: json.RawMessage(`{}`), Seed: &seed, ModelState: modelstate.DeclaredW0(config.Model.FoundationID, config.Model.Name)})
		if e != nil {
			return e
		}
		return printJSON(r)
	default:
		return fmt.Errorf("unknown runtime command %q", args[0])
	}
}
func trainingCommand(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("training requires inspect <jsonl>, export <jsonl> [-out training], or w1-dataset [-out training] <jsonl...>")
	}
	switch args[0] {
	case "inspect":
		v, e := readCandidates(args[1])
		if e != nil {
			return e
		}
		fmt.Printf("candidates=%d\n", len(v))
		return nil
	case "export":
		fs := flag.NewFlagSet("training export", flag.ContinueOnError)
		out := fs.String("out", "training", "output directory")
		if e := fs.Parse(args[2:]); e != nil {
			return e
		}
		v, e := readCandidates(args[1])
		if e != nil {
			return e
		}
		m, e := training.ExportDirectory(context.Background(), *out, v)
		if e != nil {
			return e
		}
		return printJSON(m)
	case "w1-dataset":
		fs := flag.NewFlagSet("training w1-dataset", flag.ContinueOnError)
		out := fs.String("out", "training", "output directory")
		if e := fs.Parse(args[1:]); e != nil {
			return e
		}
		paths := fs.Args()
		if len(paths) == 0 {
			return fmt.Errorf("training w1-dataset requires one or more candidate jsonl files")
		}
		var all []training.Candidate
		for _, p := range paths {
			v, e := readCandidates(p)
			if e != nil {
				return e
			}
			all = append(all, v...)
		}
		path, e := training.WriteW1Dataset(*out, all)
		if e != nil {
			return e
		}
		if path == "" {
			fmt.Println("no W1-eligible candidates in the given input; nothing written")
			return nil
		}
		fmt.Println(path)
		return nil
	default:
		return fmt.Errorf("unknown training command %q", args[0])
	}
}
func readCandidates(path string) ([]training.Candidate, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 4<<20)
	var out []training.Candidate
	for s.Scan() {
		var c training.Candidate
		if e = json.Unmarshal(s.Bytes(), &c); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, s.Err()
}
func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func printJSON(v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e == nil {
		fmt.Println(string(b))
	}
	return e
}
