package measurement

import "sort"

type Aggregate struct {
	Outcome                                         Outcome
	Supporting, Contradicting, Inconclusive, Errors int
	IndependentSources                              int
	Results                                         []Result
	Reason                                          string
}

func AggregateResults(results []Result) Aggregate {
	a := Aggregate{Outcome: NotMeasured, Results: append([]Result(nil), results...)}
	groups := map[string]bool{}
	authoritativeSupport := false
	for _, r := range results {
		switch r.Outcome {
		case Contradicted:
			a.Contradicting++
			if r.Authoritative() {
				a.Outcome = Contradicted
				a.Reason = "authoritative measurement contradicted claim"
				return a
			}
		case Supported:
			a.Supporting++
			if r.Authoritative() {
				authoritativeSupport = true
			}
		case Inconclusive, Unsupported, NotMeasured:
			a.Inconclusive++
		case Error:
			a.Errors++
		}
		for _, e := range r.Evidence {
			if e.Independent {
				key := e.CorrelationGroup
				if key == "" {
					key = e.Source
				}
				groups[key] = true
			}
		}
	}
	a.IndependentSources = len(groups)
	switch {
	case authoritativeSupport:
		a.Outcome = Supported
		a.Reason = "authoritative measurement supported claim"
	case a.Supporting > 0:
		a.Outcome = Supported
		a.Reason = "one or more measurements support claim"
	case a.Contradicting > 0:
		a.Outcome = Contradicted
		a.Reason = "measurements contradict claim"
	case a.Inconclusive > 0:
		a.Outcome = Inconclusive
		a.Reason = "available measurements are inconclusive"
	case a.Errors > 0:
		a.Outcome = Error
		a.Reason = "measurement subsystem error"
	}
	sort.SliceStable(a.Results, func(i, j int) bool { return a.Results[i].StartedAt.Before(a.Results[j].StartedAt) })
	return a
}
