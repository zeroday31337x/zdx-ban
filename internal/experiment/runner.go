package experiment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"zdx-ban/internal/ban"
	"zdx-ban/internal/inference"
	"zdx-ban/internal/measurement"
	"zdx-ban/internal/memory"
	"zdx-ban/internal/model"
	"zdx-ban/internal/telemetry"
	tr "zdx-ban/internal/trace"
)

type Runner struct {
	Provider        model.Provider
	Registry        *Registry
	Dataset         Dataset
	Config          RunConfig
	OutputRoot      string
	ExperimentID    string
	Verbose         bool
	Progress        func(PairedResult, int, int)
	MemoryRetrieval *memory.RetrievalResult
	SkipBaseline    bool
}
type trackedProvider struct {
	model.Provider
	mu               sync.Mutex
	calls            []ProviderCall
	inferenceTimeout time.Duration
}

func (p *trackedProvider) invoke(ctx context.Context, f func(context.Context) (model.GenerateResponse, error)) (model.GenerateResponse, error) {
	started := time.Now().UTC()
	if p.inferenceTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.inferenceTimeout)
		defer cancel()
	}
	resp, err := f(ctx)
	finished := time.Now().UTC()
	call := ProviderCall{StartedAt: started, FinishedAt: finished, Duration: finished.Sub(started), Completed: err == nil, Failed: err != nil, Runtime: resp.Call}
	if resp.PromptTokens > 0 {
		x := resp.PromptTokens
		call.PromptTokens = &x
	}
	if resp.CompletionTokens > 0 {
		x := resp.CompletionTokens
		call.CompletionTokens = &x
	}
	if err != nil {
		call.FailureCode = string(inference.FailureCodeOf(err))
		call.Error = err.Error()
		call.TimedOut = inference.FailureCodeOf(err) == inference.ProviderTimeout
	}
	p.mu.Lock()
	call.Attempt = len(p.calls) + 1
	p.calls = append(p.calls, call)
	p.mu.Unlock()
	return resp, err
}
func (p *trackedProvider) Generate(ctx context.Context, r model.GenerateRequest) (model.GenerateResponse, error) {
	return p.invoke(ctx, func(c context.Context) (model.GenerateResponse, error) { return p.Provider.Generate(c, r) })
}
func (p *trackedProvider) GenerateStructured(ctx context.Context, r model.GenerateRequest, d any) (model.GenerateResponse, error) {
	return p.invoke(ctx, func(c context.Context) (model.GenerateResponse, error) {
		return inference.GenerateStructured(c, p.Provider, r, d)
	})
}
func (p *trackedProvider) accounting() ProviderAccounting {
	p.mu.Lock()
	defer p.mu.Unlock()
	a := ProviderAccounting{Calls: append([]ProviderCall(nil), p.calls...)}
	for _, c := range a.Calls {
		a.Attempted++
		if c.Completed {
			a.Completed++
			a.SuccessfulResponses++
		}
		if c.Failed {
			a.Failed++
		}
		if c.TimedOut {
			a.TimedOut++
		}
	}
	return a
}

type seededProvider struct {
	model.Provider
	seed *int
}

func (p seededProvider) Generate(ctx context.Context, r model.GenerateRequest) (model.GenerateResponse, error) {
	r.Seed = p.seed
	return p.Provider.Generate(ctx, r)
}
func (p seededProvider) GenerateStructured(ctx context.Context, r model.GenerateRequest, d any) (model.GenerateResponse, error) {
	r.Seed = p.seed
	return inference.GenerateStructured(ctx, p.Provider, r, d)
}
func (r *Runner) Validate() error {
	if r.Registry == nil || r.Provider == nil {
		return fmt.Errorf("provider and verifier registry required")
	}
	if !r.Config.RequireObjectiveVerification {
		return fmt.Errorf("experimental runs require objective verification")
	}
	if r.Config.Repetitions < 1 {
		return fmt.Errorf("repetitions must be positive")
	}
	if r.Config.Model == "" || r.Config.MaxTokens < 1 || r.Config.Timeout <= 0 {
		return fmt.Errorf("invalid model configuration")
	}
	for _, c := range r.Dataset.Cases {
		if err := ValidateCase(c, r.Registry); err != nil {
			return err
		}
	}
	if r.OutputRoot == "" {
		return fmt.Errorf("output location required")
	}
	return nil
}
func (r *Runner) DryRun() error { return r.Validate() }
func (r *Runner) Run(ctx context.Context, resume bool) (ResultFile, error) {
	if err := r.Validate(); err != nil {
		return ResultFile{}, err
	}
	if r.ExperimentID == "" {
		r.ExperimentID = newID()
	}
	dir := filepath.Join(r.OutputRoot, r.ExperimentID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ResultFile{}, err
	}
	state, err := r.loadOrCreate(ctx, dir, resume)
	if err != nil {
		return state, err
	}
	done := map[string]bool{}
	for _, x := range state.Cases {
		done[pairKey(x.CaseID, x.Repetition)] = true
	}
	total := len(r.Dataset.Cases) * r.Config.Repetitions
	for rep := 1; rep <= r.Config.Repetitions; rep++ {
		for _, c := range r.Dataset.Cases {
			if done[pairKey(c.ID, rep)] {
				continue
			}
			select {
			case <-ctx.Done():
				state.Manifest.FinishedAt = nil
				_ = persist(dir, &state)
				return state, ctx.Err()
			default:
			}
			caseCtx := ctx
			cancelCase := func() {}
			if r.Config.CaseTimeout > 0 {
				caseCtx, cancelCase = context.WithTimeout(ctx, r.Config.CaseTimeout)
			}
			row := r.runPair(caseCtx, c, rep)
			cancelCase()
			if ctx.Err() != nil {
				_ = persist(dir, &state)
				return state, ctx.Err()
			}
			state.Cases = append(state.Cases, row)
			state.Summary = Summarize(state.Cases)
			if err = persist(dir, &state); err != nil {
				return state, err
			}
			if r.Progress != nil {
				r.Progress(row, len(state.Cases), total)
			}
		}
	}
	now := time.Now().UTC()
	state.Manifest.FinishedAt = &now
	state.Summary = Summarize(state.Cases)
	if err = persist(dir, &state); err != nil {
		return state, err
	}
	return state, nil
}
func (r *Runner) runPair(ctx context.Context, c Case, rep int) PairedResult {
	c.Measurement.Contract.Provenance.GitCommit = gitCommit()
	c.Measurement.Contract.Provenance.GitRemote = gitRemote()
	c.Measurement.Contract.Provenance.GitDirty = gitDirty()
	seed := r.Config.Seed
	if seed != nil {
		x := *seed + rep - 1
		seed = &x
	}
	baseTracker := &trackedProvider{Provider: r.Provider, inferenceTimeout: r.Config.InferenceTimeout}
	p := seededProvider{baseTracker, seed}
	v, _ := r.Registry.Get(c.VerifierType)
	row := PairedResult{CaseID: c.ID, Category: c.Category, Repetition: rep, Seed: seed, AttemptStartedAt: time.Now().UTC(), ConfigurationHash: configHash(r.Config)}
	before := telemetry.Capture()
	var start time.Time
	var after telemetry.Snapshot
	if !r.SkipBaseline {
		start = time.Now()
		baseResp, err := p.Generate(ctx, model.GenerateRequest{Prompt: c.Prompt, Temperature: r.Config.Temperature, Seed: seed, MaxTokens: r.Config.MaxTokens})
		after = telemetry.Capture()
		row.Baseline = SideResult{Answer: baseResp.Text, StartedAt: start.UTC(), FinishedAt: time.Now().UTC(), Latency: time.Since(start), Provider: baseTracker.accounting(), Telemetry: telemetryResult(before, after, time.Since(start), baseResp.Latency)}
		row.Baseline.ModelCalls = row.Baseline.Provider.SuccessfulResponses
		for _, call := range row.Baseline.Provider.Calls {
			row.Baseline.ProviderDuration += call.Duration
		}
		row.Baseline.Tokens = tokenPtr(baseResp)
		if err != nil {
			row.Baseline.Verification = errorVerification(ctx, err)
			row.Baseline.FailureCategory = string(row.Baseline.Verification.Outcome)
		} else {
			row.Baseline.Verification = verify(v, ctx, c, baseResp.Text)
			if !row.Baseline.Verification.Passed {
				row.Baseline.FailureCategory = string(row.Baseline.Verification.Outcome)
			}
		}
		row.Baseline.VerificationDuration = row.Baseline.Verification.Duration
		row.Baseline.OrchestrationDuration = row.Baseline.Latency - row.Baseline.ProviderDuration - row.Baseline.VerificationDuration
		if row.Baseline.OrchestrationDuration < 0 {
			row.Baseline.OrchestrationDuration = 0
		}
		if row.Baseline.Verification.Measurement.ID != "" {
			row.Baseline.Measurements = append(row.Baseline.Measurements, row.Baseline.Verification.Measurement)
		}
	}
	before = telemetry.Capture()
	start = time.Now()
	banTracker := &trackedProvider{Provider: r.Provider, inferenceTimeout: r.Config.InferenceTimeout}
	p = seededProvider{banTracker, seed}
	e := ban.NewEngine(p, r.Config.BAN)
	e.Temperature = r.Config.Temperature
	e.MaxTokens = tokenBudget(r.Config.FinalAnswerMaxTokens, r.Config.MaxTokens)
	e.Generator.Temperature = r.Config.Temperature
	e.Generator.MaxTokens = tokenBudget(r.Config.ProposalMaxTokens, r.Config.MaxTokens)
	e.Evaluator.Temperature = r.Config.Temperature
	e.Evaluator.MaxTokens = tokenBudget(r.Config.EvaluationMaxTokens, r.Config.MaxTokens)
	e.Evaluator.ChallengeMaxTokens = tokenBudget(r.Config.ChallengeMaxTokens, r.Config.MaxTokens)
	e.Verifier = BranchVerifier{c, v}
	e.TraceDir = ""
	if r.MemoryRetrieval != nil {
		e.Executor = ArithmeticExecutor{}
		guide := memory.Guidance(*r.MemoryRetrieval)
		e.Generator.MemoryGuide = guide
		ids := []string{}
		reasons := map[string]any{}
		for _, x := range r.MemoryRetrieval.Records {
			ids = append(ids, x.Record.ID)
			reasons[x.Record.ID] = x.Reason
		}
		for _, well := range r.MemoryRetrieval.GravityWells {
			strength := well.BaseStrength
			decayHalfLife := well.DecayHalfLife
			if strength == 0 && well.Strength > 0 {
				// Compatibility for retrieval snapshots written before wells kept
				// their unmodulated evidence strength separately.
				strength = well.Strength
				decayHalfLife = 0
			}
			e.GravityWells = append(e.GravityWells, ban.GravityWell{ID: well.ID, Category: well.Category, Strength: strength, Repulsion: well.Repulsion, DecayHalfLife: decayHalfLife, LastUsedAt: well.LastUsedAt, SuccessRate: well.SuccessRate, ApplicationCount: well.ApplicationCount, OutcomeCount: well.OutcomeCount, SupportingEvidence: well.SupportingEvidence, FoundationAnchor: well.FoundationAnchor, Keywords: append([]string(nil), well.Keywords...)})
		}
		e.GravityWeight = r.Config.MemoryRetrieval.Gravity.Weight
		e.Memory = ban.MemoryInteraction{Enabled: true, InitialSnapshotHash: r.MemoryRetrieval.SnapshotHash, RetrievedMemoryIDs: ids, RetrievalReasons: reasons, WorkingMemoryChars: r.MemoryRetrieval.ApproxChars, GuidanceHash: configHash(guide), GravityWells: append([]ban.GravityWell(nil), e.GravityWells...)}
	}
	result, bt, berr := e.Run(ctx, c.Prompt)
	after = telemetry.Capture()
	row.BAN = SideResult{StartedAt: start.UTC(), FinishedAt: time.Now().UTC(), Latency: time.Since(start), Provider: banTracker.accounting(), Telemetry: telemetryResult(before, after, time.Since(start), 0)}
	row.BAN.ModelCalls = row.BAN.Provider.SuccessfulResponses
	for _, call := range row.BAN.Provider.Calls {
		row.BAN.ProviderDuration += call.Duration
	}
	row.BAN.GraphSearchDuration = row.BAN.Latency - row.BAN.ProviderDuration
	if row.BAN.GraphSearchDuration < 0 {
		row.BAN.GraphSearchDuration = 0
	}
	if bt != nil {
		if bt.Metrics.Tokens > 0 {
			x := bt.Metrics.Tokens
			row.BAN.Tokens = &x
		}
		row.InitialWinnerID = bt.InitialTopBranch
		row.FinalWinnerID = bt.SelectedBranch
		row.TraceRunID = bt.RunID
		row.Memory = bt.Memory
		row.Graph = graphMetrics(bt)
		row.BANInitial = initialVerification(ctx, bt, v, c)
		if row.BANInitial.Measurement.ID != "" {
			row.CandidateMeasurements = append(row.CandidateMeasurements, row.BANInitial.Measurement)
		}
		for _, n := range bt.Nodes {
			row.CandidateMeasurements = append(row.CandidateMeasurements, n.Measurements...)
		}
		row.RecoveryAttempted = row.BANInitial.Measurement.Outcome == measurement.Contradicted
		row.RecoverySuccessful = row.RecoveryAttempted && bt.RecoveredFromWrongBranch
	}
	if berr != nil {
		row.BAN.Verification = errorVerification(ctx, berr)
		row.BAN.FailureCategory = classifyBAN(berr, bt)
		row.CompletedAt = time.Now().UTC()
		return row
	}
	row.BAN.Answer = result.Answer
	if result.Selected != nil {
		row.SelectedRoute = BranchRoute{ID: result.Selected.ID, Title: result.Selected.Title, ReasoningSummary: result.Selected.ReasoningSummary, Assumptions: append([]string(nil), result.Selected.Assumptions...), GravityWellID: result.Selected.GravityWellID, InformationGravity: result.Selected.InformationGravity, GravityRepulsion: result.Selected.GravityRepulsion}
	}
	row.BAN.Verification = verify(v, ctx, c, result.Answer)
	row.FinalMeasurements = append(row.FinalMeasurements, row.BAN.Verification.Measurement)
	row.BAN.Measurements = append(row.BAN.Measurements, row.BAN.Verification.Measurement)
	row.RecoverySuccessful = row.RecoveryAttempted && row.BAN.Verification.Passed && bt.RecoveredFromWrongBranch
	if !row.BAN.Verification.Passed {
		row.BAN.FailureCategory = classifyFinal(row.BAN.Verification, bt)
	}
	row.CompletedAt = time.Now().UTC()
	row.BAN.VerificationDuration = row.BAN.Verification.Duration
	row.BAN.OrchestrationDuration = row.BAN.Latency - row.BAN.ProviderDuration - row.BAN.VerificationDuration
	if row.BAN.OrchestrationDuration < 0 {
		row.BAN.OrchestrationDuration = 0
	}
	return row
}

func tokenBudget(specific, fallback int) int {
	if specific > 0 {
		return specific
	}
	return fallback
}
func (r *Runner) loadOrCreate(ctx context.Context, dir string, resume bool) (ResultFile, error) {
	path := filepath.Join(dir, "summary.json")
	if resume {
		b, err := os.ReadFile(path)
		if err != nil {
			return ResultFile{}, err
		}
		var s ResultFile
		if err = json.Unmarshal(b, &s); err != nil {
			return s, err
		}
		if s.Manifest.DatasetSHA256 != r.Dataset.SHA256 || configHash(s.Manifest.Configuration) != configHash(r.Config) {
			return s, fmt.Errorf("resume configuration or dataset drift")
		}
		return s, nil
	}
	info := model.Info{Provider: r.Config.Provider, Model: r.Config.Model}
	if p, ok := r.Provider.(interface {
		ModelInfo(context.Context) (model.Info, error)
	}); ok {
		if reported, err := p.ModelInfo(ctx); err == nil {
			info = reported
		}
	}
	return ResultFile{Experiment: ExperimentName, SchemaVersion: SchemaVersion, Manifest: Manifest{Experiment: ExperimentName, SchemaVersion: SchemaVersion, ExperimentID: r.ExperimentID, DatasetVersion: r.Dataset.Version, DatasetPath: r.Dataset.Path, DatasetSHA256: r.Dataset.SHA256, GitCommit: gitCommit(), GitRemote: gitRemote(), GitDirty: gitDirty(), TraceSchemaVersion: ban.TraceSchemaVersion, MemorySchemaVersion: memory.SchemaVersion, Model: info, Configuration: r.Config, Host: telemetry.Capture(), StartedAt: time.Now().UTC()}}, nil
}
func persist(dir string, s *ResultFile) error {
	if _, err := tr.WriteAtomic(dir, "summary", s); err != nil {
		return err
	}
	if _, err := tr.WriteAtomic(dir, "manifest", s.Manifest); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".paired-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	enc := json.NewEncoder(f)
	for _, row := range s.Cases {
		if err = enc.Encode(row); err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(dir, "paired-results.jsonl")); err != nil {
		return err
	}
	ok = true
	return WriteReport(filepath.Join(dir, "report.md"), s)
}
func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s-%s", ExperimentName, time.Now().UTC().Format("20060102-150405"), hex.EncodeToString(b))
}
func pairKey(id string, r int) string { return fmt.Sprintf("%s#%d", id, r) }
func configHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func gitRemote() string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	b, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	return string(bytesTrim(b))
}
func gitDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	b, err := cmd.Output()
	return err != nil || len(bytesTrim(b)) > 0
}
func gitCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	b, err := cmd.Output()
	if err != nil {
		return "unavailable"
	}
	return string(bytesTrim(b))
}
func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}
func telemetryResult(a, b telemetry.Snapshot, d, ml time.Duration) RunTelemetry {
	peak := a.HeapAlloc
	if b.HeapAlloc > peak {
		peak = b.HeapAlloc
	}
	return RunTelemetry{Before: a, After: b, PeakHeapAlloc: peak, TotalAllocBefore: a.TotalAlloc, TotalAllocAfter: b.TotalAlloc, GCBefore: a.NumGC, GCAfter: b.NumGC, Duration: d, ModelLatency: ml}
}
func tokenPtr(r model.GenerateResponse) *int {
	x := r.PromptTokens + r.CompletionTokens
	if x == 0 {
		return nil
	}
	return &x
}
func errorVerification(ctx context.Context, err error) Verification {
	m := measurement.Result{ID: fmt.Sprintf("error-%d", time.Now().UnixNano()), Outcome: measurement.Error, VerificationClass: measurement.Unverified, Authority: measurement.UnknownAuthority, Independence: measurement.UnknownIndependence, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), Error: &measurement.MeasurementError{Code: string(inference.FailureCodeOf(err)), Message: err.Error()}}
	switch inference.FailureCodeOf(err) {
	case inference.ModelOutputMalformed:
		return Verification{Outcome: MalformedOutput, Details: err.Error(), Measurement: m}
	case inference.ProviderTimeout:
		return Verification{Outcome: TimedOut, Details: err.Error(), Measurement: m}
	case inference.ProviderCancelled:
		return Verification{Outcome: Interrupted, Details: err.Error(), Measurement: m}
	case inference.VerificationError:
		return Verification{Outcome: VerifierError, Details: err.Error(), Measurement: m}
	case inference.NoVerifiedCandidate:
		return Verification{Outcome: NoVerifiedCandidate, Details: err.Error(), Measurement: m}
	default:
		return Verification{Outcome: ProviderFailure, Details: err.Error(), Measurement: m}
	}
}
func initialVerification(ctx context.Context, t *ban.ExecutionTrace, v ObjectiveVerifier, c Case) Verification {
	for _, n := range t.Nodes {
		if n.ID == t.InitialTopBranch {
			return verify(v, ctx, c, n.Hypothesis)
		}
	}
	return Verification{Outcome: Incorrect, Details: "initial winner missing from trace"}
}
func graphMetrics(t *ban.ExecutionTrace) GraphMetrics {
	g := GraphMetrics{CandidateProposals: t.Metrics.CandidateProposals, NodesCreated: len(t.Nodes), DuplicateProposals: t.Metrics.DuplicateProposals, DuplicatesDetected: t.Metrics.DuplicateProposals, AnswerConvergences: t.Metrics.AnswerConvergences, DiversityRegenerations: t.Metrics.DiversityRegenerations, BranchesPruned: t.Metrics.PrunedNodes, PrunedCandidates: t.Metrics.PrunedNodes, ProviderEvaluations: t.Metrics.ProviderEvaluations, PeakActiveBranches: t.Metrics.PeakActiveBranches, GravityRoutedBranches: t.Metrics.GravityRoutedBranches, GravityWellHits: t.Metrics.GravityWellHits, GravityRecoveryAttempts: t.Metrics.GravityRecoveryAttempts, GravityRecoveries: t.Metrics.GravityRecoveries}
	for _, n := range t.Nodes {
		if n.Depth > g.MaxDepthReached {
			g.MaxDepthReached = n.Depth
		}
		if len(n.ParentIDs) > 1 {
			g.MultipleParentNodes++
			g.Convergences++
		}
	}
	return g
}
func classifyBAN(err error, t *ban.ExecutionTrace) string {
	if code := inference.FailureCodeOf(err); code != "" {
		return string(code)
	}
	return string(inference.NoVerifiedCandidate)
}

func contains(s, q string) bool {
	for i := 0; i+len(q) <= len(s); i++ {
		if s[i:i+len(q)] == q {
			return true
		}
	}
	return false
}
func classifyFinal(v Verification, t *ban.ExecutionTrace) string {
	if v.Outcome == MalformedOutput {
		return "model_output_malformed"
	}
	if t != nil && t.SelectedBranch != "" {
		return "selected_branch_final_answer_incorrect"
	}
	return string(v.Outcome)
}

var _ = runtime.GOARCH
