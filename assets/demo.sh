#!/usr/bin/env bash
# demo.sh: the session recorded in assets/demo.gif
#
# Regenerate (asciinema 2.x and agg):
#   asciinema rec --cols 104 --rows 25 -c assets/demo.sh demo.cast
#   agg --theme monokai --font-size 16 --idle-time-limit 3 demo.cast assets/demo.gif
#
# 104x25 renders close to 16:9.

set -e
repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'kill "$agent" 2>/dev/null || true; rm -rf "$work"' EXIT

(cd "$repo/examples/live" && go build -o "$work/agent" .)
cd "$work"
export PATH="$work:$PATH"
mkdir prompts
printf 'model: claude-opus-5\ntemperature: 0.2\nworkers: 4\n' > config.yaml
echo "You are a careful assistant." > prompts/system.md

say() { printf '\033[38;5;244m# %s\033[0m\n' "$1"; sleep 0.8; }

run() {
	printf '\033[1;32m$\033[0m '
	for ((i = 0; i < ${#1}; i++)); do
		printf '%s' "${1:i:1}"
		sleep 0.035
	done
	sleep 0.4
	echo
	eval "$1" || true
	sleep "${2:-1.8}"
}

clear
say "argus: a running program that follows its configuration"
run "cat config.yaml" 1.2
run "AGENT_WORKERS=8 agent &" 2.5
agent=$!
say "edit a value: one new revision, no restart"
run "sed -i 's/0.2/0.7/' config.yaml" 2.2
echo
say "save a broken file: refused, the last good revision keeps serving"
run "echo 'model: [unclosed' >> config.yaml" 2.8
run "sed -i '\$d' config.yaml" 1.5
echo
say "prompts are documents, read as text and watched like the rest"
run "echo 'Be brief.' > prompts/tone.md" 3
