# Ubuntu W0/W1 cron watchdog

This Ubuntu-native launcher selects either the resumable W0 full-weight worker
or the W1 LoRA worker through `ZDX_TRAINING_STAGE`. It does not depend on Termux.
The cron job starts one process when none is alive, retries exit 75 low-memory
pauses and unexpected process deaths, and resumes from the last hashed
optimizer-step checkpoint.

A killed process cannot checkpoint after it is dead. At worst, work since the
last completed optimizer-step checkpoint is lost. W1 writes every optimizer
step by default; the W0 pilot defaults to every ten steps because each
checkpoint contains the full model and optimizer. W0 persists model, optimizer,
scheduler, RNG, immutable release identity, and exact packed-corpus cursor.

## Configure and preflight

Create an isolated Python virtual environment with the required PyTorch,
Transformers, PEFT, and safetensors packages. Then edit the ignored local file:

```bash
cp training/slow_ubuntu/training.env.example training/slow_ubuntu/training.env
chmod 600 training/slow_ubuntu/training.env
```

Leave `ZDX_TRAINING_ENABLED=0` while configuring. Select `W0` or `W1`, use an
immutable release and a fresh stage-specific output directory, then run the
matching preflight explicitly.

W0:

```bash
"$ZDX_PYTHON" training/w0/trainer.py \
  --release "$ZDX_W0_RELEASE" \
  --output "$ZDX_SLOW_OUTPUT" \
  --preflight
```

W1:

```bash
source training/slow_ubuntu/training.env
"$ZDX_PYTHON" training/slow_android/trainer.py \
  --release "$ZDX_W1_RELEASE" \
  --base-model "$ZDX_W0_TRANSFORMERS" \
  --output "$ZDX_SLOW_OUTPUT" \
  --config "$ZDX_W1_CONFIG" \
  --preflight
```

Only after preflight succeeds, set `ZDX_TRAINING_ENABLED=1` and start once with:

```bash
training/slow_ubuntu/resume.sh
```

## Cron and operations

Install an idempotent five-minute user crontab entry:

```bash
training/slow_ubuntu/install-cron.sh
```

The installer preserves unrelated crontab lines. Check `cron` is enabled by the
host (`sudo systemctl enable --now cron` on a normal Ubuntu system); containers
and restricted VPS environments may not run systemd even when `crontab` exists.

```bash
training/slow_ubuntu/status.sh
training/slow_ubuntu/stop.sh
training/slow_ubuntu/resume.sh
training/slow_ubuntu/uninstall-cron.sh
```

Exit code 2 means a permanent configuration, dependency, release-integrity, or
state error. The runner creates `PERMANENT_FAILURE`, and cron will not loop on
it. Inspect the log and state, fix the cause, then use `resume.sh` to clear the
latch. A `STOP` marker and final/complete state also prevent restarts.

`W0_TRAINING_COMPLETE_UNVALIDATED` and `TRAINING_COMPLETE_UNVALIDATED` remain
unregistered and untrusted until their respective acceptance gates pass. W1
must bind to the exact accepted W0 tree hash; never point W1 at an in-progress
W0 output.
