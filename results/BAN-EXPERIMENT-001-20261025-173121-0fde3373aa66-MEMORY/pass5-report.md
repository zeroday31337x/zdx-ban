# BAN Pass 5 Memory Validation

Raw observations: 80  

## Outcome Distribution

- BASELINE: supported=9 contradicted=11 inconclusive=0 unsupported=0 unknown=0 failed=0
- BAN_COLD: supported=7 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=13
- BAN_MEMORY: supported=9 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=11
- BAN_MISLEADING_MEMORY: supported=10 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=10

## Memory Utility and Harm

Retrieved records: 290  
Helpful: 0  
Harmful: 0  
Recovered from harm: 1  
Rejected memories: 6  
Authoritative overrides: 10  
Retrieval precision (score >= 0.20): 99.31%  
Retrieved but unused: 249  
Retrieval usefulness: 0.00%  
Harmful retrieval rate: 0.00%  
Harm recovery rate: 2.50%  
Measurement-contract compliant: 46/80  
Novel candidates preserved: 3  
Branches avoided: 11  
Model calls avoided: 9  

## Comparisons

- BASELINE -> BAN_COLD: paired=7 improved=5 regressed=0 unchanged=2
- BAN_COLD -> BAN_MEMORY: paired=6 improved=0 regressed=0 unchanged=6
- BAN_MEMORY -> BAN_MISLEADING_MEMORY: paired=8 improved=0 regressed=0 unchanged=8

## Per-Category Results

### arithmetic_constraints

- BASELINE: supported=1 contradicted=3 inconclusive=0
- BAN_COLD: supported=2 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=4 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=4 contradicted=0 inconclusive=0
### coding_debugging

- BASELINE: supported=0 contradicted=4 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### forced_recovery

- BASELINE: supported=4 contradicted=0 inconclusive=0
- BAN_COLD: supported=1 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=1 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=2 contradicted=0 inconclusive=0
### logic_deduction

- BASELINE: supported=0 contradicted=4 inconclusive=0
- BAN_COLD: supported=4 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=3 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=4 contradicted=0 inconclusive=0
### structured_transformation

- BASELINE: supported=4 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=1 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0

## Limitations

Memory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.
