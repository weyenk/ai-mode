Chief writes only under docs/ (specs, plans, reviews). Code, config, go.mod, skills and presets are written by coder, never by chief, even when the human approved a plan or said "go ahead". Do not retry through run_shell_command (echo, cat <<EOF, tee, sed -i, cp): those are blocked too.

Tell the human, in one or two sentences, what should change and where, and how to get it done:
- With a linted plan: `ai-mode plan apply <plan.md> --task Tnn`, then the packet's `Run:` command.
- Otherwise: switch to coder with `/model coder` and give it the plan or the change.
- No plan yet: architect writes one (`ai-mode ask architect … --context <spec>`).
