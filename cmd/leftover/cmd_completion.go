package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/MaiSamayTha/leftover/internal/app"
)

func init() {
	register(command{name: "completion", args: "<bash|zsh|fish|powershell>", short: "print a shell completion script", run: cmdCompletion})
}

// completionFlags lists each command's flags for tab completion. The test
// checks every one against the command's real flags.
var completionFlags = map[string][]string{
	"scan":          {"--json", "-q", "--all", "--min", "--idle", "--tool"},
	"clean":         {"--yes", "--json", "--all", "--min", "--idle", "--tool"},
	"explore":       {"-n", "--json"},
	"why":           {"--json"},
	"docker":        {"--json"},
	"docker prune":  {"--images", "--yes", "--json"},
	"vault":         {"--json"},
	"vault restore": {"--strict", "--last"},
	"vault empty":   {"--expired", "--yes"},
	"history":       {"--json", "-n"},
	"rules":         {"--json", "--here"},
	"update":        {"--check", "--yes", "--json"},
	"report":        {"--json", "-n", "--idle"},
}

// valueFlags take a value, so nothing is suggested right after them.
var valueFlags = map[string]bool{"--min": true, "--idle": true, "--tool": true, "-n": true}

var subcommands = map[string][]string{
	"vault":      {"list", "restore", "empty"},
	"ignore":     {"list", "add", "remove"},
	"docker":     {"prune"},
	"completion": {"bash", "zsh", "fish", "powershell"},
}

// completeDirs tells the shell script to complete folder names itself.
const completeDirs = ":dirs"

// complete returns what could come next on the command line. prior holds
// the words after "leftover" before the one being typed, cur.
func complete(cur string, prior []string) []string {
	if len(prior) == 0 {
		var out []string
		for _, c := range commands {
			out = append(out, c.name)
		}
		return filterPrefix(append(out, "help"), cur)
	}
	if valueFlags[prior[len(prior)-1]] {
		return nil
	}
	cmd, rest := prior[0], positional(prior[1:])
	key := cmd
	if len(rest) > 0 && (cmd == "vault" || cmd == "docker") {
		key = cmd + " " + rest[0]
		rest = rest[1:]
	}
	if strings.HasPrefix(cur, "-") {
		return filterPrefix(completionFlags[key], cur)
	}
	if subs, ok := subcommands[cmd]; ok && key == cmd && len(rest) == 0 {
		return filterPrefix(subs, cur)
	}
	switch key {
	case "explore", "why", "ignore":
		return []string{completeDirs}
	case "vault restore", "vault empty":
		return filterPrefix(vaultIDs(), cur)
	case "clean":
		return filterPrefix(itemIDs(), cur)
	}
	return nil
}

// positional drops flags and their values from words.
func positional(words []string) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		w := words[i]
		if strings.HasPrefix(w, "-") {
			if valueFlags[w] && !strings.Contains(w, "=") {
				i++
			}
			continue
		}
		out = append(out, w)
	}
	return out
}

func filterPrefix(words []string, prefix string) []string {
	var out []string
	for _, w := range words {
		if strings.HasPrefix(w, prefix) {
			out = append(out, w)
		}
	}
	sort.Strings(out)
	return out
}

func vaultIDs() []string {
	a, err := app.Open()
	if err != nil {
		return nil
	}
	entries, _ := a.Vault.List()
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

func itemIDs() []string {
	a, err := app.Open()
	if err != nil {
		return nil
	}
	last := a.Last()
	if last == nil {
		return nil
	}
	var out []string
	for _, it := range last.Items {
		if it.Cleanable() {
			out = append(out, it.ID)
		}
	}
	return out
}

// cmdComplete is what the shell scripts call: leftover __complete --cur=<word> <words before it...>
func cmdComplete(args []string) error {
	cur := ""
	if len(args) > 0 && strings.HasPrefix(args[0], "--cur=") {
		cur, args = strings.TrimPrefix(args[0], "--cur="), args[1:]
	}
	for _, w := range complete(cur, args) {
		fmt.Println(w)
	}
	return nil
}

func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return usageError("usage: leftover completion bash|zsh|fish|powershell")
	}
	script, ok := completionScripts[args[0]]
	if !ok {
		return usageError("no completion for %q; use bash, zsh, fish or powershell", args[0])
	}
	_, err := os.Stdout.WriteString(script)
	return err
}

var completionScripts = map[string]string{
	"bash": `# Leftover completion for bash. Add to ~/.bashrc:
#   eval "$(leftover completion bash)"
_leftover() {
  local cur=${COMP_WORDS[COMP_CWORD]} out
  out=$(leftover __complete "--cur=$cur" "${COMP_WORDS[@]:1:COMP_CWORD-1}" 2>/dev/null)
  if [[ $out == ":dirs" ]]; then
    COMPREPLY=($(compgen -d -- "$cur"))
  else
    COMPREPLY=($(compgen -W "$out" -- "$cur"))
  fi
}
complete -o filenames -F _leftover leftover
`,
	"zsh": `#compdef leftover
# Leftover completion for zsh. Add to ~/.zshrc:
#   source <(leftover completion zsh)
_leftover() {
  local out
  out=$(leftover __complete "--cur=${words[CURRENT]}" "${(@)words[2,CURRENT-1]}" 2>/dev/null)
  if [[ $out == ":dirs" ]]; then
    _files -/
  else
    local -a c
    c=("${(@f)out}")
    compadd -a c
  fi
}
if [[ $funcstack[1] == _leftover ]]; then
  _leftover "$@"
else
  compdef _leftover leftover
fi
`,
	"fish": `# Leftover completion for fish. Save to ~/.config/fish/completions/leftover.fish:
#   leftover completion fish > ~/.config/fish/completions/leftover.fish
function __leftover_complete
    set -l prior (commandline -opc)[2..-1]
    set -l out (leftover __complete "--cur="(commandline -ct) $prior 2>/dev/null)
    if test "$out" = ":dirs"
        __fish_complete_directories (commandline -ct)
    else
        printf '%s\n' $out
    end
end
complete -c leftover -f -a '(__leftover_complete)'
`,
	"powershell": `# Leftover completion for PowerShell. Add to your $PROFILE:
#   leftover completion powershell | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName leftover -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $prior = @($commandAst.CommandElements | Select-Object -Skip 1 | ForEach-Object { $_.ToString() })
    if ($wordToComplete -ne '' -and $prior.Count -gt 0) { $prior = @($prior | Select-Object -SkipLast 1) }
    $out = @(& leftover __complete "--cur=$wordToComplete" @prior 2>$null)
    if ($out.Count -eq 1 -and $out[0] -eq ':dirs') {
        Get-ChildItem -Directory -Path "$wordToComplete*" -ErrorAction SilentlyContinue | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_.FullName, $_.Name, 'ProviderContainer', $_.FullName)
        }
    } else {
        $out | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
    }
}
`,
}
