# BAN configuration

`ban.config` is the strict, versioned JSON configuration for the model-facing
BAN runtime. Startup rejects missing files, unknown fields, unsupported schema
versions, invalid URLs and durations, unsafe response limits, and inconsistent
search bounds.

The configuration controls:

- Ollama provider name, deployed model name, declared W0 identity, endpoint,
  connection timeout, retry count, and maximum streamed response size;
- temperature and token budgets for baseline, proposal, evaluation, challenge,
  and final-answer calls;
- branch/search and concurrency bounds;
- runtime timeout, trace directory, benchmark dataset, and memory path;
- default experiment temperatures, token budgets, timeouts, repetitions, seed,
  and BAN search bounds.

Durations are Go duration strings such as `10s`, `5m`, or `1h`. Relative paths
are resolved from the process working directory.

## Selection and precedence

The default file is `./ban.config`. Select another file before the command:

```bash
./bin/ban --config /etc/zdx/ban.config run "problem"
```

`BAN_CONFIG` selects a different file. For compatibility, these environment
variables override values after the file is loaded:

- `BAN_MODEL` overrides `model.name`;
- `OLLAMA_BASE_URL` overrides `model.base_url`;
- `BAN_MEMORY_FILE` overrides `runtime.memory_file`.

Command flags override the resolved configuration for that invocation. The old
`--concurrency N` flag still overrides both model-call and evaluation
concurrency. `--model-concurrency` and `--evaluation-concurrency` can override
them independently.

Inspect the fully resolved configuration, including environment overrides:

```bash
./bin/ban config show
./bin/ban config validate
```

Every experiment still records its resolved run configuration in its manifest;
changing `ban.config` cannot silently alter a resumed experiment because resume
validation rejects configuration drift.

## Boundary with W0 training

`ban.config` configures the BAN runtime and its deployed model. It is not the W0
Transformers architecture or training policy. The separate `training/w0`
release contract freezes and hashes its corpus, architecture, tokenizer, and
training configuration. After a W0 passes validation and is deployed, place its
deployment name and immutable release identity in `model.name` and
`model.foundation_id`.
