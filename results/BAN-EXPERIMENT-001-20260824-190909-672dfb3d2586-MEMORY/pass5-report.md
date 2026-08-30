# BAN Pass 5 Memory Validation

Raw observations: 72  

## Outcome Distribution

- BASELINE: supported=7 contradicted=11 inconclusive=0 unsupported=0 unknown=0 failed=0
- BAN_COLD: supported=4 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=14
- BAN_MEMORY: supported=6 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=12
- BAN_MISLEADING_MEMORY: supported=9 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=9

## Memory Utility and Harm

Retrieved records: 54  
Helpful: 0  
Harmful: 0  
Recovered from harm: 0  
Rejected memories: 3  
Authoritative overrides: 9  
Retrieval precision (score >= 0.20): 88.89%  
Retrieved but unused: 51  
Retrieval usefulness: 0.00%  
Harmful retrieval rate: 0.00%  
Harm recovery rate: 0.00%  
Measurement-contract compliant: 37/72  
Novel candidates preserved: 1  
Branches avoided: 4  
Model calls avoided: 3  

## Comparisons

- BASELINE -> BAN_COLD: paired=4 improved=3 regressed=0 unchanged=1
- BAN_COLD -> BAN_MEMORY: paired=3 improved=0 regressed=0 unchanged=3
- BAN_MEMORY -> BAN_MISLEADING_MEMORY: paired=5 improved=0 regressed=0 unchanged=5

## Per-Category Results

### arithmetic_constraints

- BASELINE: supported=1 contradicted=3 inconclusive=0
- BAN_COLD: supported=1 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=2 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=2 contradicted=0 inconclusive=0
### coding_debugging

- BASELINE: supported=0 contradicted=4 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### forced_recovery

- BASELINE: supported=2 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=1 contradicted=0 inconclusive=0
### logic_deduction

- BASELINE: supported=0 contradicted=4 inconclusive=0
- BAN_COLD: supported=3 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=3 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=3 contradicted=0 inconclusive=0
### structured_transformation

- BASELINE: supported=4 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=1 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=3 contradicted=0 inconclusive=0

## Limitations

Memory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.
