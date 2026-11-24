# Live

A running program that follows its configuration: the program recorded in the
demo at the top of the [README](../../README.md).

```bash
go build -o agent .
cd my-agent        # a directory with config.yaml and prompts/*.md
/path/to/agent
```

It reads `config.yaml`, the `AGENT_*` environment variables and the prompts in
`prompts/*.md`, prints where each value came from, then one line per revision.
Edit the files while it runs: a change is a new revision, a broken file is
refused and the previous revision keeps serving. [assets/demo.sh](../../assets/demo.sh)
records exactly that.
