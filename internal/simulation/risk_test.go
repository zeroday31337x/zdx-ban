package simulation

import (
 "math"
 "testing"
)

func TestAnalyzeRisk(t *testing.T) {
 s:=Scenario{Title:"Options",Choices:[]Choice{
  {ID:"a",Label:"A",Branches:[]Branch{
   {Label:"up",Probability:0.75,Score:4},
   {Label:"down",Probability:0.25,Score:-6},
  }},
  {ID:"b",Label:"B",Branches:[]Branch{
   {Label:"start",Probability:1,Score:1,Next:[]Branch{
    {Label:"ok",Probability:0.5,Score:2},
    {Label:"bad",Probability:0.5,Score:-2},
   }},
  }},
 }}
 reports,err:=AnalyzeRisk(s,0)
 if err!=nil{t.Fatal(err)}
 if len(reports)!=2||reports[0].ChoiceID!="a"||reports[1].ChoiceID!="b"{t.Fatalf("unexpected ranking: %+v",reports)}
 a,b:=reports[0],reports[1]
 if math.Abs(a.ProbabilityBelowThreshold-.25)>1e-10||a.ExpectedShortfall!=6||a.WorstCase.Score!=-6||a.MostLikely.Steps[0]!="up"{t.Fatalf("unexpected a: %+v",a)}
 if math.Abs(b.ProbabilityBelowThreshold-.5)>1e-10||b.ExpectedShortfall!=1||len(b.WorstCase.Steps)!=2{t.Fatalf("unexpected b: %+v",b)}
}
func TestAnalyzeRiskRejectsInvalid(t *testing.T) {
 s:=Scenario{Title:"T",Choices:[]Choice{{ID:"a",Label:"A",Branches:[]Branch{{Label:"x",Probability:1,Score:0}}}}}
 for _,v:=range []float64{math.NaN(),math.Inf(1),1e10}{
  if _,err:=AnalyzeRisk(s,v);err==nil{t.Fatalf("accepted invalid threshold %v",v)}
 }
 s.Choices[0].Branches[0].Probability=.3
 if _,err:=AnalyzeRisk(s,0);err==nil{t.Fatal("accepted invalid tree")}
}
