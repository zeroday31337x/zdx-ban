package simulation

import (
 "fmt"
 "math"
 "sort"
)

// Outcome is a terminal scenario path with its assumed joint probability.
// Steps retain the sequence of events leading to the outcome.
type Outcome struct {
 Steps []string `json:"steps"`
 Probability float64 `json:"probability"`
 Score float64 `json:"score"`
}

// RiskReport provides transparent, assumption-based tail risk and scenario paths.
// The loss threshold is a cumulative score, not a currency amount.
type RiskReport struct {
 ChoiceID string `json:"choice_id"`
 LossThreshold float64 `json:"loss_threshold"`
 ProbabilityBelowThreshold float64 `json:"probability_below_threshold"`
 ExpectedShortfall float64 `json:"expected_shortfall"`
 MostLikely Outcome `json:"most_likely"`
 WorstCase Outcome `json:"worst_case"`
 BestCase Outcome `json:"best_case"`
}

// AnalyzeRisk enumerates bounded paths and returns conditional tail-loss severity.
// It uses caller-provided probabilities; it is not a calibrated forecast.
func AnalyzeRisk(s Scenario, threshold float64) ([]RiskReport, error) {
 if math.IsNaN(threshold) || math.IsInf(threshold, 0) || math.Abs(threshold)>1e9 { return nil, fmt.Errorf("invalid risk threshold") }
 if _,err:=Evaluate(s);err!=nil{return nil,err}
 reports:=make([]RiskReport,0,len(s.Choices))
 for _,choice:=range s.Choices {
  paths:=make([]Outcome,0)
  if err:=collectOutcomes(choice.Branches,1,1,0,nil,&paths);err!=nil{return nil,err}
  r:=RiskReport{ChoiceID:choice.ID,LossThreshold:threshold,MostLikely:paths[0],WorstCase:paths[0],BestCase:paths[0]}
  severity:=0.0
  for _,p:=range paths {
   if p.Probability>r.MostLikely.Probability {r.MostLikely=p}
   if p.Score<r.WorstCase.Score {r.WorstCase=p}
   if p.Score>r.BestCase.Score {r.BestCase=p}
   if p.Score<threshold {
    r.ProbabilityBelowThreshold+=p.Probability
    severity+=p.Probability*(threshold-p.Score)
   }
  }
  if r.ProbabilityBelowThreshold>0 {r.ExpectedShortfall=severity/r.ProbabilityBelowThreshold}
  reports=append(reports,r)
 }
 sort.Slice(reports,func(i,j int)bool{
  if reports[i].ProbabilityBelowThreshold==reports[j].ProbabilityBelowThreshold{return reports[i].ChoiceID<reports[j].ChoiceID}
  return reports[i].ProbabilityBelowThreshold<reports[j].ProbabilityBelowThreshold
 })
 return reports,nil
}

func collectOutcomes(branches []Branch, depth int, probability, score float64, steps []string, out *[]Outcome) error {
 if depth>maxDepth || len(branches)==0 || len(branches)>maxFanout{return fmt.Errorf("scenario tree budget exceeded")}
 for _,b:=range branches {
  path:=make([]string,len(steps)+1)
  copy(path,steps)
  path[len(steps)]=b.Label
  p,s:=probability*b.Probability,score+b.Score
  if len(b.Next)>0 {
   if err:=collectOutcomes(b.Next,depth+1,p,s,path,out);err!=nil{return err}
  } else {
   if len(*out)>=maxPaths{return fmt.Errorf("terminal path budget exceeded")}
   *out=append(*out,Outcome{Steps:path,Probability:p,Score:s})
  }
 }
 return nil
}
