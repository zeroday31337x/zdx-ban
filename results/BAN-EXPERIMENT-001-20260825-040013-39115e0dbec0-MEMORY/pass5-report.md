# BAN Pass 5 Memory Validation

Raw observations: 20  

## Outcome Distribution

- BASELINE: supported=1 contradicted=4 inconclusive=0 unsupported=0 unknown=0 failed=0
- BAN_COLD: supported=3 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=2
- BAN_MEMORY: supported=5 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=0
- BAN_MISLEADING_MEMORY: supported=5 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=0

## Memory Utility and Harm

Retrieved records: 50  
Helpful: 0  
Harmful: 0  
Recovered from harm: 1  
Rejected memories: 3  
Authoritative overrides: 5  
Retrieval precision (score >= 0.20): 96.00%  
Retrieved but unused: 33  
Retrieval usefulness: 0.00%  
Harmful retrieval rate: 0.00%  
Harm recovery rate: 10.00%  
Measurement-contract compliant: 18/20  
Novel candidates preserved: 3  
Branches avoided: 1  
Model calls avoided: 3  

## Comparisons

- BASELINE -> BAN_COLD: paired=3 improved=2 regressed=0 unchanged=1
- BAN_COLD -> BAN_MEMORY: paired=3 improved=0 regressed=0 unchanged=3
- BAN_MEMORY -> BAN_MISLEADING_MEMORY: paired=5 improved=0 regressed=0 unchanged=5

## Per-Category Results

### arithmetic_constraints

- BASELINE: supported=1 contradicted=3 inconclusive=0
- BAN_COLD: supported=2 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=4 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=4 contradicted=0 inconclusive=0
### logic_deduction

- BASELINE: supported=0 contradicted=1 inconclusive=0
- BAN_COLD: supported=1 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=1 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=1 contradicted=0 inconclusive=0

## Limitations

Memory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.
