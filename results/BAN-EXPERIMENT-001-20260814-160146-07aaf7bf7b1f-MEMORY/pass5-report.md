# BAN Pass 5 Memory Validation

Raw observations: 80  

## Outcome Distribution

- BASELINE: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=20
- BAN_COLD: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=20
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=20
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0 unsupported=0 unknown=0 failed=20

## Memory Utility and Harm

Retrieved records: 60  
Helpful: 0  
Harmful: 0  
Recovered from harm: 0  
Rejected memories: 0  
Authoritative overrides: 0  
Retrieval precision (score >= 0.20): 90.00%  
Retrieved but unused: 60  
Retrieval usefulness: 0.00%  
Harmful retrieval rate: 0.00%  
Harm recovery rate: 0.00%  
Measurement-contract compliant: 0/80  
Novel candidates preserved: 0  
Branches avoided: 0  
Model calls avoided: 0  

## Comparisons

- BASELINE -> BAN_COLD: paired=20 improved=0 regressed=0 unchanged=20
- BAN_COLD -> BAN_MEMORY: paired=20 improved=0 regressed=0 unchanged=20
- BAN_MEMORY -> BAN_MISLEADING_MEMORY: paired=20 improved=0 regressed=0 unchanged=20

## Per-Category Results

### arithmetic_constraints

- BASELINE: supported=0 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### coding_debugging

- BASELINE: supported=0 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### forced_recovery

- BASELINE: supported=0 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### logic_deduction

- BASELINE: supported=0 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0
### structured_transformation

- BASELINE: supported=0 contradicted=0 inconclusive=0
- BAN_COLD: supported=0 contradicted=0 inconclusive=0
- BAN_MEMORY: supported=0 contradicted=0 inconclusive=0
- BAN_MISLEADING_MEMORY: supported=0 contradicted=0 inconclusive=0

## Limitations

Memory-use attribution is based on observable retrieval and condition deltas, not private chain-of-thought. Better memory performance does not prove that remembered information is true. It demonstrates that historical information improved measured behavior under the recorded experimental contract. A harmful-memory recovery result is valuable: BAN is judged by whether current evidence lets it escape incorrect historical assumptions.
