# Settings

`argus.Setup` in a running program: one call declaring where configuration
comes from, and a handle that stays current.

```bash
go run .
```

The example writes a small application into a temporary directory — a
`config.json` and a `prompts/system.md` — then, while you watch:

1. saves two files together, and one revision comes out;
2. saves a broken `config.json`, which is refused: the previous revision keeps
   serving and `OnError` says why;
3. fixes it, and the next cycle recovers.

It prints `Explain()` as JSON: which revision this instance runs, and where
each value comes from.

The directory is printed at startup. Edit anything in it while the example
runs — a value, a new prompt, a deleted one. Ctrl-C stops it.

## In main.go

- the single `Setup(...)` chain
- `Bind[Config]`, a struct kept in step with the configuration
- `FileIfPresent`, for an override that may or may not be deployed
- `OnReload` receiving `Change.Keys` and `Change.Documents`
- `Doc("prompts", "system")`: text read as text, in its own namespace
- `MaxStaleness`, a latency budget

## Related

- [API reference](../../docs/API-REFERENCE.md#setup-and-settings)
- [Quick start](../../docs/quick-start.md)
