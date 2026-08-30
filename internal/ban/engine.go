package ban

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/telemetry"
	tr "zdx-ban/internal/trace"
)

type Engine struct {
	Provider      inference.Engine
	Generator     Generator
	Evaluator     Evaluator
	Verifier      Verifier
	Config        Config
	TraceDir      string
	Temperature   float64
	MaxTokens     int
	Logf          func(string, ...any)
	Memory        MemoryInteraction
	GravityWells  []GravityWell
	GravityWeight float64
	Executor      CapabilityExecutor
}

func NewEngine(p inference.Engine, c Config) *Engine {
	return &Engine{Provider: p, Generator: Generator{Provider: p, Temperature: .2, MaxTokens: 1024}, Evaluator: Evaluator{Provider: p, Temperature: .1, MaxTokens: 384, ChallengeMaxTokens: 256}, Verifier: AcceptVerifier{}, Config: c, TraceDir: "traces", Temperature: .2, MaxTokens: 256, GravityWeight: .20, Logf: func(string, ...any) {}}
}
func (e *Engine) Run(ctx context.Context, goal string) (Result, *ExecutionTrace, error) {
	if goal == "" {
		return Result{}, nil, errors.New("goal is required")
	}
	start := time.Now().UTC()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", goal, start.UnixNano())))
	runID := hex.EncodeToString(sum[:8])
	g := NewGraph(e.Config.MaxDepth, e.Config.MaxNodes)
	t := &ExecutionTrace{TraceSchemaVersion: TraceSchemaVersion, RunID: runID, Problem: goal, Config: e.Config, StartedAt: start, RuntimeStart: telemetry.Capture(), Memory: e.Memory}
	defer func() {
		t.Nodes = g.Nodes()
		t.Edges = g.Edges()
		t.Memory.GravityWells = cloneGravityWells(e.GravityWells)
		t.FinishedAt = time.Now().UTC()
		t.RuntimeFinish = telemetry.Capture()
		t.Metrics.TotalNodes = len(t.Nodes)
		t.Metrics.Latency = t.FinishedAt.Sub(t.StartedAt)
	}()
	if p, ok := e.Provider.(inference.ModelStateReporter); ok {
		if info, err := p.ModelState(ctx); err == nil {
			t.Model = info
		}
	} else if p, ok := e.Provider.(interface {
		ModelInfo(context.Context) (inference.ModelInfo, error)
	}); ok {
		if info, err := p.ModelInfo(ctx); err == nil {
			t.Model = info
		}
	}
	e.Logf("[BAN] Goal received")
	props, resp, err := e.Generator.Generate(ctx, goal, e.Config.InitialBranches, nil)
	if err != nil {
		return Result{}, t, err
	}
	addUsage(&t.Metrics, resp)
	t.Metrics.CandidateProposals += len(props)
	initial := make([]*State, 0, len(props))
	for i, p := range props {
		s := NewState(fmt.Sprintf("b%02d", i+1), p, 0)
		n, dup, er := g.AddNode(s)
		if er != nil {
			return Result{}, t, er
		}
		if dup {
			t.Metrics.DuplicateProposals++
		}
		if !dup {
			initial = append(initial, n)
			if n.Metadata["answer_convergence_with"] != "" {
				t.Metrics.AnswerConvergences++
			}
		}
	}
	if len(initial) < e.Config.RetainBranches {
		missing := e.Config.RetainBranches - len(initial)
		more, retryResp, retryErr := e.Generator.Generate(ctx, goal, missing, initial)
		if retryErr != nil {
			return Result{}, t, retryErr
		}
		addUsage(&t.Metrics, retryResp)
		t.Metrics.DiversityRegenerations++
		t.Metrics.CandidateProposals += len(more)
		for _, p := range more {
			s := NewState(fmt.Sprintf("b%02d", len(g.Nodes())+1), p, 0)
			n, dup, addErr := g.AddNode(s)
			if addErr != nil {
				return Result{}, t, addErr
			}
			if dup {
				t.Metrics.DuplicateProposals++
				continue
			}
			initial = append(initial, n)
			if n.Metadata["answer_convergence_with"] != "" {
				t.Metrics.AnswerConvergences++
			}
		}
	}
	e.Logf("[BAN] Generated %d branches", len(initial))
	if len(initial) < e.Config.RetainBranches {
		return Result{}, t, errors.New("insufficient distinct branches")
	}
	if err = e.evaluate(ctx, goal, initial, &t.Metrics); err != nil {
		return Result{}, t, err
	}
	rank(initial)
	viable := filterViable(initial)
	if len(viable) == 0 {
		return Result{}, t, errors.New("all branches rejected")
	}
	t.InitialTopBranch = viable[0].ID
	retained := append([]*State(nil), viable[:min(e.Config.RetainBranches, len(viable))]...)
	if len(viable) > len(retained) && len(retained) < e.Config.MaxActiveBranches {
		retained = append(retained, viable[len(retained)])
	}
	t.Metrics.PeakActiveBranches = len(retained)
	keep := map[string]bool{}
	for _, s := range retained {
		keep[s.ID] = true
		s.Status = Active
	}
	for _, s := range initial {
		if !keep[s.ID] {
			s.Status = Pruned
		}
	}
	var children []*State
	for _, parent := range retained[:min(e.Config.RetainBranches, len(retained))] {
		if parent.Depth >= e.Config.MaxDepth {
			continue
		}
		ps, r, er := e.Generator.Generate(ctx, goal, 2, []*State{parent})
		if er != nil {
			return Result{}, t, er
		}
		addUsage(&t.Metrics, r)
		t.Metrics.CandidateProposals += len(ps)
		for _, p := range ps {
			s := NewState(fmt.Sprintf("b%02d", len(g.Nodes())+1), p, parent.Depth+1)
			n, dup, er := g.AddNode(s)
			if er != nil {
				continue
			}
			if er = g.AddEdge(parent.ID, n.ID); er != nil {
				return Result{}, t, er
			}
			if dup {
				t.Metrics.DuplicateProposals++
			}
			if !dup {
				children = append(children, n)
				if n.Metadata["answer_convergence_with"] != "" {
					t.Metrics.AnswerConvergences++
				}
			}
		}
		parent.Status = Expanded
		t.Metrics.ExpandedNodes++
	}
	if len(children) > 0 {
		if err = e.evaluate(ctx, goal, children, &t.Metrics); err != nil {
			return Result{}, t, err
		}
	}
	candidates := append(retained, children...)
	rank(candidates)
	candidates = filterViable(candidates)
	// A measured arithmetic well can route the strategy through a local
	// deterministic executor. The model still supplies the strategy; the
	// ordinary verifier remains the sole authority for acceptance.
	if e.Executor != nil && hasArithmeticWell(e.GravityWells) {
		if answer, execErr := e.Executor.Execute(ctx, goal, "arithmetic strategy"); execErr == nil && answer != "" {
			state := NewState(fmt.Sprintf("b%02d", len(g.Nodes())+1), Proposal{Title: "deterministic arithmetic", Hypothesis: answer, ReasoningSummary: "verified arithmetic executor", Assumptions: []string{"explicit operation structure"}}, 0)
			state.Metadata["capability_executor"] = "arithmetic"
			if node, duplicate, addErr := g.AddNode(state); addErr == nil && !duplicate {
				candidates = append(candidates, node)
				t.Metrics.CandidateProposals++
				node.CapabilityInvocations = append(node.CapabilityInvocations, CapabilityInvocation{Capability: "DETERMINISTIC_ARITHMETIC", Why: "arithmetic gravity well", At: time.Now().UTC()})
			}
		}
	}
	for _, s := range candidates[:min(2, len(candidates))] {
		ch, r, er := e.Evaluator.Challenge(ctx, goal, s)
		if er != nil {
			return Result{}, t, er
		}
		addUsage(&t.Metrics, r)
		s.Challenge = ch
	}
	var winner *State
	for _, s := range candidates {
		vr := e.Verifier.Verify(ctx, goal, s)
		s.VerificationResults = append(s.VerificationResults, vr)
		if !vr.Passed {
			outcome := measurement.Contradicted
			if len(s.Measurements) > 0 {
				outcome = s.Measurements[len(s.Measurements)-1].Outcome
			}
			if outcome != measurement.Contradicted {
				s.Status = Evaluated
				continue
			}
			s.Status = Failed
			UpdateGravityOutcome(e.GravityWells, s, time.Now().UTC())
			if s.ID == t.InitialTopBranch {
				t.RecoveredFromWrongBranch = true
				t.RecoveryDepth = s.Depth
				t.ReasonForSwitch = vr.Details
			}
			continue
		}
		s.Status = Verified
		UpdateGravityOutcome(e.GravityWells, s, time.Now().UTC())
		winner = s
		break
	}
	if winner == nil && len(e.GravityWells) > 0 && e.Config.GravityRecoveryBranches > 0 {
		t.Metrics.GravityRecoveryAttempts++
		recoveryProposals, recoveryResponse, recoveryErr := e.Generator.Generate(ctx, goal, e.Config.GravityRecoveryBranches, candidates)
		if recoveryErr != nil {
			return Result{}, t, recoveryErr
		}
		addUsage(&t.Metrics, recoveryResponse)
		t.Metrics.CandidateProposals += len(recoveryProposals)
		recoveryStates := make([]*State, 0, len(recoveryProposals))
		for _, proposal := range recoveryProposals {
			state := NewState(fmt.Sprintf("b%02d", len(g.Nodes())+1), proposal, 1)
			node, duplicate, addErr := g.AddNode(state)
			if addErr != nil {
				return Result{}, t, addErr
			}
			if duplicate {
				t.Metrics.DuplicateProposals++
				continue
			}
			for _, rejected := range candidates {
				if edgeErr := g.AddEdge(rejected.ID, node.ID); edgeErr != nil {
					return Result{}, t, edgeErr
				}
			}
			recoveryStates = append(recoveryStates, node)
		}
		if len(recoveryStates) > 0 {
			if evaluateErr := e.evaluate(ctx, goal, recoveryStates, &t.Metrics); evaluateErr != nil {
				return Result{}, t, evaluateErr
			}
			rank(recoveryStates)
			for _, state := range filterViable(recoveryStates) {
				verification := e.Verifier.Verify(ctx, goal, state)
				state.VerificationResults = append(state.VerificationResults, verification)
				if !verification.Passed {
					state.Status = Failed
					UpdateGravityOutcome(e.GravityWells, state, time.Now().UTC())
					continue
				}
				state.Status = Verified
				UpdateGravityOutcome(e.GravityWells, state, time.Now().UTC())
				winner = state
				t.Metrics.GravityRecoveries++
				t.RecoveredFromWrongBranch = true
				t.RecoveryDepth = state.Depth
				t.ReasonForSwitch = "gravity recovery regenerated after deterministic rejection"
				break
			}
		}
	}
	if winner == nil {
		return Result{}, t, errors.New("no candidate passed verification")
	}
	winner.Status = Selected
	t.SelectedBranch = winner.ID
	t.RecoveredFromWrongBranch = t.RecoveredFromWrongBranch || winner.ID != t.InitialTopBranch
	// The candidate has already passed the authoritative verifier. Return it
	// verbatim; a second model generation could corrupt a correct answer.
	result := Result{Answer: winner.Hypothesis, Selected: winner}
	t.Nodes = g.Nodes()
	for _, n := range t.Nodes {
		t.MeasurementEvents = append(t.MeasurementEvents, n.Measurements...)
	}
	t.Edges = g.Edges()
	t.Result = result
	t.FinishedAt = time.Now().UTC()
	t.RuntimeFinish = telemetry.Capture()
	t.Metrics.TotalNodes = len(t.Nodes)
	t.Metrics.DidRecover = t.RecoveredFromWrongBranch
	t.Metrics.Latency = t.FinishedAt.Sub(t.StartedAt)
	for _, s := range t.Nodes {
		if s.Status == Pruned {
			t.Metrics.PrunedNodes++
		}
	}
	t.Memory.GravityWells = cloneGravityWells(e.GravityWells)
	if e.TraceDir != "" {
		if _, err = tr.WriteAtomic(e.TraceDir, t.RunID, t); err != nil {
			return result, t, err
		}
	}
	e.Logf("[BAN] Selected %s", winner.ID)
	return result, t, nil
}
func (e *Engine) evaluate(ctx context.Context, goal string, states []*State, m *Metrics) error {
	type pair struct {
		E Evaluation
		R inference.Result
	}
	pairs, err := Parallel(ctx, e.Config.MaxConcurrentEvaluations, states, func(c context.Context, s *State) (pair, error) {
		v, r, er := e.Evaluator.Evaluate(c, goal, s)
		return pair{v, r}, er
	})
	if err != nil {
		return err
	}
	for i, p := range pairs {
		ApplyEvaluation(states[i], p.E)
		if ApplyGravity(states[i], e.GravityWells, e.GravityWeight) {
			m.GravityRoutedBranches++
			m.GravityWellHits++
		}
		addUsage(m, p.R)
	}
	return nil
}
func addUsage(m *Metrics, r inference.Result) {
	m.ModelCalls++
	m.ProviderEvaluations++
	m.Tokens += r.PromptTokens + r.CompletionTokens
}
func rank(s []*State) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].AggregateScore > s[j].AggregateScore })
}
func filterViable(s []*State) []*State {
	out := make([]*State, 0, len(s))
	for _, v := range s {
		if v.Status != Pruned && v.Status != Failed {
			out = append(out, v)
		}
	}
	return out
}

func hasArithmeticWell(wells []GravityWell) bool {
	for _, well := range wells {
		category := strings.ToLower(well.Category)
		if strings.Contains(category, "arithmetic") || strings.Contains(category, "numeric") {
			return true
		}
		for _, keyword := range well.Keywords {
			keyword = strings.ToLower(keyword)
			if keyword == "arithmetic" || keyword == "numeric" || keyword == "calculate" {
				return true
			}
		}
	}
	return false
}

func cloneGravityWells(wells []GravityWell) []GravityWell {
	cloned := append([]GravityWell(nil), wells...)
	for i := range cloned {
		cloned[i].Keywords = append([]string(nil), cloned[i].Keywords...)
	}
	return cloned
}
