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

## Multi-stage trajectories and sensitivity

The evaluator also reports `risk_analysis` for each choice:

- `expected_score_by_stage`: probability-weighted cumulative score after each stage. A path that has already terminated carries its final score forward to the remaining stages of that choice.
- `p10_score`, `median_score`, `p90_score`: discrete weighted score quantiles. Quantiles depend on the given input distribution, not on calibrated real-world odds.
- `worst_10_percent_mean`: mean of the lowest 10% of outcome-probability mass, splitting a terminal path's probability when needed. This is a lower-tail expectation, not a confidence interval.
- `worst_paths` and `most_likely_paths`: at most three concise terminal-path explanations, with conditional probability products and cumulative stage scores.

Optional `stress_delta` (0 to 0.25, default zero) performs a **one-pair probability-shift sensitivity test**. At *each* branching point, it moves up to that amount of conditional probability from the highest expected-score child to the lowest expected-score child, using stressed child values recursively. The report includes `pessimistic_shift_expected_score` and a separate `stress_ranking`.

This sensitivity test **is not a proven worst-case bound** and is not a substitute for probability intervals, correlated uncertainty modeling, or real-world calibration. It is a transparent way to see whether a small, intentionally pessimistic change to the stated assumptions reverses the recommended ordering. Zero disables the additional shift calculation.

Example input is in `examples/decision-scenario.json`. No model weights, provider credentials, private inference services, or network calls are involved.
