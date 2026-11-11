# Standalone scenario evaluation

BAN's deterministic scenario evaluator compares explicitly specified multi-step outcomes. It is **not** an LLM, a Monte Carlo predictor, or a substitute for independently measured probabilities.

Run:

```sh
go run ./cmd/ban scenario examples/decision-scenario.json
```

Each choice contains a tree of branches. A branch has a label, a conditional probability, an additive score, and optionally `next` branches for subsequent stages. Sibling conditional probabilities must total 1 within 0.000001. Every complete path receives the product of its branch probabilities and the sum of its branch scores.

The output includes each choice's expected score, probability that the total score is negative, best/worst possible path scores, and number of terminal paths, ranked by expected score. Results are explicitly tagged `ASSUMPTION_BASED` and `USER_SUPPLIED`; changing the assumptions changes the ranking. The engine does not learn probabilities or execute actions.

Safety/resource limits: maximum 32 choices, 32 branches at each level, depth 8, 8192 terminal paths per choice, and a 1 MiB input file. Nonfinite numbers, malformed probabilities, unknown JSON fields, and trailing JSON are rejected. It only reads the given local JSON path and produces a report.

For objective forecasts, probabilities must be grounded in observed frequencies or calibrated models, recorded with provenance and uncertainty. No production inference provider, external network, or ZeroDriveX private service is used.
