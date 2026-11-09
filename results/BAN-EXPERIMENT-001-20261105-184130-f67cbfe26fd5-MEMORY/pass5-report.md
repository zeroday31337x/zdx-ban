# BAN Pass 5 Memory Validation

Raw observations: 180  

## Outcome Distribution

- BASELINE: supported=23 contradicted=20 inconclusive=2 unsupported=0 unknown=0 failed=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=45
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=45
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=45

## Memory Utility and Harm

Retrieved records: 225  
Helpful: 0  
Harmful: 0  
Recovered from harm: 0  
Rejected memories: 0  
Authoritative overrides: 0  
Retrieval precision (score >= 0.20): 94.67%  
Retrieved but unused: 225  
Retrieval usefulness: 0.00%  
Harmful retrieval rate: 0.00%  
Harm recovery rate: 0.00%  
Measurement-contract compliant: 45/180  
Novel candidates preserved: 3  
Branches avoided: 0  
Model calls avoided: 0  

## Comparisons

- BASELINE -> BAN_COLD: paired=0 improved=0 regressed=0 unchanged=0
- BAN_COLD -> BAN_MEMORY: paired=0 improved=0 regressed=0 unchanged=0
- BAN_MEMORY -> BAN_MISLEADING_MEMORY: paired=0 improved=0 regressed=0 unchanged=0

## Per-Category Results

### arithmetic_constraints

- BASELINE: supported=18 contradicted=0 inconclusive=2
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### logic_deduction

- BASELINE: supported=0 contradicted=20 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### structured_transformation

- BASELINE: supported=5 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0

## Limitations

Memory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.
