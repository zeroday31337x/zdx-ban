package ban

import "testing"

func TestGraphConvergenceCyclesDedupAndPrune(t *testing.T) {
	g := NewGraph(2, 5)
	a := NewState("a", Proposal{Title: "A", Hypothesis: "alpha"}, 0)
	b := NewState("b", Proposal{Title: "B", Hypothesis: "beta"}, 1)
	c := NewState("c", Proposal{Title: "C", Hypothesis: "gamma"}, 1)
	d := NewState("d", Proposal{Title: "D", Hypothesis: "delta"}, 2)
	for _, s := range []*State{a, b, c, d} {
		if _, _, e := g.AddNode(s); e != nil {
			t.Fatal(e)
		}
	}
	for _, e := range [][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}} {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	if len(d.ParentIDs) != 2 {
		t.Fatalf("parents=%v", d.ParentIDs)
	}
	if g.AddEdge("d", "a") == nil {
		t.Fatal("cycle accepted")
	}
	dup := NewState("x", Proposal{Title: " D ", Hypothesis: "DELTA"}, 2)
	got, isDup, err := g.AddNode(dup)
	if err != nil || !isDup || got.ID != "d" {
		t.Fatalf("dedup: %v %v %v", got, isDup, err)
	}
	if err = g.Prune("c"); err != nil || c.Status != Pruned {
		t.Fatal("prune failed")
	}
}
func TestGraphLimits(t *testing.T) {
	g := NewGraph(0, 1)
	if _, _, e := g.AddNode(NewState("a", Proposal{Title: "a", Hypothesis: "a"}, 0)); e != nil {
		t.Fatal(e)
	}
	if _, _, e := g.AddNode(NewState("b", Proposal{Title: "b", Hypothesis: "b"}, 0)); e == nil {
		t.Fatal("node limit ignored")
	}
	g2 := NewGraph(0, 2)
	if _, _, e := g2.AddNode(NewState("x", Proposal{Title: "x", Hypothesis: "x"}, 1)); e == nil {
		t.Fatal("depth ignored")
	}
}

func TestDistinctPathsMayConvergeOnSameAnswer(t *testing.T) {
	g := NewGraph(1, 5)
	a := NewState("a", Proposal{Title: "Direct", Hypothesis: "45", ReasoningSummary: "multiply then add"}, 0)
	b := NewState("b", Proposal{Title: "Algebra", Hypothesis: "45", ReasoningSummary: "solve symbolically"}, 0)
	if _, dup, err := g.AddNode(a); err != nil || dup {
		t.Fatalf("first path: dup=%v err=%v", dup, err)
	}
	got, dup, err := g.AddNode(b)
	if err != nil || dup {
		t.Fatalf("distinct path collapsed: dup=%v err=%v", dup, err)
	}
	if got.Metadata["answer_convergence_with"] != "a" {
		t.Fatalf("answer convergence not recorded: %#v", got.Metadata)
	}
}
