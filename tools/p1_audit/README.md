# Isolated recording performance audit

Run from this repository with Go, Docker and the cached local Linux arm64 worker/MinIO images plus postgres:17-alpine, redis:7-alpine and rabbitmq:4.2.9-management. The worker image needs FFmpeg and ffprobe.

```sh
python3 tools/p1_audit/recording_pod.py --phase before --baseline-ref 86bded3 --output tmp/p1-recording-before
python3 tools/p1_audit/recording_pod.py --phase after --output tmp/p1-recording-after --rabbit
go tool pprof -top tmp/p1-recording-after/recording/recording-cpu.pprof
```

Each output directory must be new. The runner cross-compiles tests, creates its own internal pod network and ephemeral PostgreSQL/Redis/MinIO/RabbitMQ, and invokes two concurrent full recording pipelines. No host ports or user data volumes are used. `--phase before` overlays only `internal/infrastructure/composite/capture.go` from the given Git ref; live sources are unchanged. Overlay source files end in `.go.txt`.

It saves profiles, resource samples and source/image identifiers. Optional `--rabbit` additionally benchmarks confirmed commands and reconnect. Tests retain encoded sources for validation until their fixture cleanup; `/proc` samples are lower bounds and may miss short FFmpeg children or final cleanup. Parent CPU/heap include API, SFU, clients, recorder and fixture authentication work, not only production recording.

Cleanup verifies the runner's unique label before removing its own containers with `rm -v`, then removes its own internal network and checks anonymous-volume/container/network leftovers. Pre-existing container IDs/StartedAt are recorded for comparison. Real decoded-media/recovery/auto-stop checks must complete in both rooms; skipped workloads fail the runner.
