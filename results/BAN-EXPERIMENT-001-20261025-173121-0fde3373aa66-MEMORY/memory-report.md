# BAN Memory Experiment

Dataset: ban-memory-exp-001-smoke-v2  
Dataset hash: `1130b53f7a6a9e7757d96ef8b9984963c762ece977d2cb092d10dbc810edea10`  
Cases: 20  
Memory schema: 0.1  
Initial memory hash: `583f5d7a04b809d838386ed2c48f6388d43f2784e6fe2419aded9dd600399960`  
Final memory hash: `27531701f1f05e262fbcf7972735868d756e9f3c52bfbf7f399702428349c01d`

## Measurement outcomes

- BASELINE: supported=9 contradicted=11 inconclusive=0 errors=0
- BAN_COLD: supported=7 contradicted=0 inconclusive=0 errors=13
- BAN_MEMORY: supported=9 contradicted=0 inconclusive=0 errors=11
- BAN_MISLEADING_MEMORY: supported=10 contradicted=0 inconclusive=0 errors=10

## Memory behavior

Retrieved records in memory condition: 145  
Relevant hits: 141  
Potentially harmful records retrieved: 22  
Measured memory-induced errors: 0  
Contradiction events: 11  
Supersessions: 0  
Episodic writes: 9  
Consolidations: 3  
Gravity wells routed: 50  
Verified gravity evidence: 106  
Peak gravity strength: 0.584  
Branches routed by gravity: 92  
Memory leverage (branches avoided; comparable completed pairs only): 11  
Memory leverage (model calls avoided; comparable completed pairs only): 9

## Gravity trajectory

- memory-001: strength=0.000 evidence=0 wells=1 routed-branches=4 memory-outcome=SUPPORTED
- memory-002: strength=0.317 evidence=1 wells=1 routed-branches=5 memory-outcome=SUPPORTED
- memory-003: strength=0.280 evidence=2 wells=1 routed-branches=4 memory-outcome=SUPPORTED
- memory-004: strength=0.267 evidence=3 wells=1 routed-branches=4 memory-outcome=SUPPORTED
- memory-005: strength=0.261 evidence=4 wells=2 routed-branches=1 memory-outcome=SUPPORTED
- memory-006: strength=0.356 evidence=5 wells=2 routed-branches=3 memory-outcome=SUPPORTED
- memory-007: strength=0.509 evidence=6 wells=2 routed-branches=4 memory-outcome=SUPPORTED
- memory-008: strength=0.584 evidence=7 wells=2 routed-branches=0 memory-outcome=ERROR
- memory-009: strength=0.584 evidence=7 wells=3 routed-branches=6 memory-outcome=ERROR
- memory-010: strength=0.584 evidence=7 wells=3 routed-branches=8 memory-outcome=SUPPORTED
- memory-011: strength=0.584 evidence=8 wells=3 routed-branches=6 memory-outcome=ERROR
- memory-012: strength=0.584 evidence=8 wells=3 routed-branches=6 memory-outcome=ERROR
- memory-013: strength=0.584 evidence=8 wells=4 routed-branches=3 memory-outcome=ERROR
- memory-014: strength=0.584 evidence=3 wells=2 routed-branches=5 memory-outcome=ERROR
- memory-015: strength=0.584 evidence=8 wells=4 routed-branches=6 memory-outcome=ERROR
- memory-016: strength=0.583 evidence=8 wells=4 routed-branches=1 memory-outcome=ERROR
- memory-017: strength=0.337 evidence=5 wells=3 routed-branches=7 memory-outcome=ERROR
- memory-018: strength=0.337 evidence=5 wells=3 routed-branches=5 memory-outcome=ERROR
- memory-019: strength=0.337 evidence=5 wells=3 routed-branches=6 memory-outcome=SUPPORTED
- memory-020: strength=0.393 evidence=6 wells=3 routed-branches=8 memory-outcome=ERROR

## Interpretation

These are measurement-relative outcomes within a controlled memory intervention. Historical memory is guidance, not current evidence. No universal intelligence or causal claim follows without live paired results and review.
