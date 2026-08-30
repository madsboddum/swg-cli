package main

import (
	"flag"
	"fmt"
	"io"
)

const completionUsage = `usage: swg completion bash|zsh|fish

Print a shell script that wires up TAB completion for swg, subcommands,
flags, -archive names, and paths inside the archives included:

    $ swg cat str<TAB>
    $ swg cat string/<TAB>
    string/en/  string/ja/
    $ swg cat string/en/ba<TAB>
    badge_d.stf  badge_n.stf

Install it once, then restart the shell or source the file:

    # bash
    swg completion bash > /usr/local/etc/bash_completion.d/swg

    # zsh
    swg completion zsh > "${fpath[1]}/_swg"

    # fish
    swg completion fish > ~/.config/fish/completions/swg.fish
`

func runCompletion(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, completionUsage) }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	operands := fs.Args()
	if len(operands) != 1 {
		fmt.Fprint(stderr, completionUsage)
		return 2
	}

	script, ok := completionScripts[operands[0]]
	if !ok {
		fmt.Fprintf(stderr, "swg completion: unknown shell %q; want bash, zsh, or fish\n", operands[0])
		return 2
	}

	fmt.Fprint(stdout, script)
	return 0
}

var completionScripts = map[string]string{
	"bash": bashCompletion,
	"zsh":  zshCompletion,
	"fish": fishCompletion,
}

// bashCompletion shells out to "swg __complete" for every candidate but
// -dir's value, which is left to bash's own directory completion. -o nospace
// lets it add the trailing space itself, so a directory candidate can leave
// the cursor ready to descend on the next TAB.
const bashCompletion = `# bash completion for swg
_swg_complete() {
    local cur=${COMP_WORDS[COMP_CWORD]}
    local prev=${COMP_WORDS[COMP_CWORD-1]}

    if [[ $prev == "-dir" ]]; then
        COMPREPLY=($(compgen -d -- "$cur"))
        return
    fi

    local IFS=$'\n'
    local words=("${COMP_WORDS[@]:1:COMP_CWORD}")
    local candidates=($(swg __complete "${words[@]}"))

    COMPREPLY=()
    local c
    for c in "${candidates[@]}"; do
        if [[ $c == */ ]]; then
            COMPREPLY+=("$c")
        else
            COMPREPLY+=("$c ")
        fi
    done
}
complete -o nospace -F _swg_complete swg
`

// zshCompletion mirrors the bash script: directories get no trailing space so
// the next TAB descends into them, everything else does.
const zshCompletion = `#compdef swg

_swg() {
    local -a words_after
    words_after=("${(@)words[2,CURRENT]}")

    if [[ ${words_after[-2]} == "-dir" ]]; then
        _path_files -/
        return
    fi

    local -a lines
    lines=("${(@f)$(swg __complete "${words_after[@]}")}")

    local -a dirs leaves c
    for c in "${lines[@]}"; do
        [[ -z $c ]] && continue
        if [[ $c == */ ]]; then
            dirs+=("$c")
        else
            leaves+=("$c")
        fi
    done

    compadd -S '' -- "${dirs[@]}"
    compadd -- "${leaves[@]}"
}
_swg
`

// fishCompletion passes the command line's own words to __complete; fish
// already treats a trailing slash as an invitation to keep completing.
const fishCompletion = `# fish completion for swg
function __swg_complete
    set -l tokens (commandline -opc) (commandline -ct)
    swg __complete $tokens[2..-1]
end
complete -c swg -f -a '(__swg_complete)'
`
