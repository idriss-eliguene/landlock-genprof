# SPO syscall-coverage compatibility

landlock-genprof optionally consumes the
`spo.x-k8s.io/syscall-coverage` annotation on merged SPO
`SeccompProfile` objects. This annotation is not covered by SPO's stability
guarantee; the importer therefore treats it as a versioned compatibility
boundary rather than a stable API.

Supported schema: `v1`.

```json
{
  "version": "v1",
  "total": 3,
  "syscalls": {
    "read": 3,
    "write": 1
  }
}
```

`total` is the number of partial SeccompProfiles in the merge group.
`syscalls[name]` is the number of those partial profiles containing that
syscall. The importer normalizes valid v1 content and records one of four
states: `ABSENT`, `KNOWN`, `MALFORMED`, or `UNSUPPORTED`.

Coverage is optional provenance. Missing, malformed, or unsupported coverage
does not invalidate an otherwise valid merged policy. It is not contributor
lineage, invocation frequency, confidence, `TrainingHistory`, authorization,
or enforcement evidence. Raw annotation bytes are not retained in the
governed authorization subject.
