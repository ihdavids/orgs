package snip

// The shell's half: a key that finds a snippet and puts it, filled in, on the
// command line - where you can still edit it, and from where it goes into your
// history like anything you typed - and `snip-prev`, which saves the command
// you just ran.
//
//	eval "$(orgs snip shell zsh)"     in .zshrc
//	eval "$(orgs snip shell bash)"    in .bashrc
//	orgs snip shell fish | source     in config.fish
//
// The key is ctrl-s, as pet's is, with terminal flow control (which owns
// ctrl-s) turned off so it reaches the shell. ORGS_SNIP_KEY picks another.

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const zshScript = `# orgs snip: %[2]s puts a snippet on the command line; snip-prev saves the last command.
# Its chatter is kept off the prompt; a failure's last line is shown, a
# cancel (esc) says nothing.
_orgs_snip_widget() {
  local cmd rc err=$(mktemp)
  cmd=$(%[1]s snip print -q "$LBUFFER" </dev/tty 2>"$err"); rc=$?
  if (( rc == 0 )) && [[ -n $cmd ]]; then BUFFER=$cmd; CURSOR=${#BUFFER}; fi
  zle reset-prompt
  (( rc != 0 && rc != 130 )) && [[ -s $err ]] && zle -M "$(tail -n 1 "$err")"
  rm -f "$err"
}
zle -N _orgs_snip_widget
[[ -t 0 ]] && stty -ixon 2>/dev/null
bindkey '%[3]s' _orgs_snip_widget
# zsh adds a line to its history after running it, so -1 is the one before.
snip-prev() { %[1]s snip new "$@" -- "$(fc -ln -1 -1)"; }
`

const bashScript = `# orgs snip: %[2]s puts a snippet on the command line; snip-prev saves the last command.
_orgs_snip_widget() {
  local cmd rc err=$(mktemp)
  cmd=$(%[1]s snip print -q "$READLINE_LINE" </dev/tty 2>"$err"); rc=$?
  if (( rc == 0 )) && [[ -n $cmd ]]; then READLINE_LINE=$cmd; READLINE_POINT=${#cmd}
  elif (( rc != 130 )) && [[ -s $err ]]; then tail -n 1 "$err" >&2; fi
  rm -f "$err"
}
[[ -t 0 ]] && stty -ixon 2>/dev/null
bind -x '"%[3]s": _orgs_snip_widget'
snip-prev() { %[1]s snip new "$@" -- "$(HISTTIMEFORMAT= history 2 | head -n 1 | sed -E 's/^ *[0-9]+ +//')"; }
`

const fishScript = `# orgs snip: %[2]s puts a snippet on the command line; snip-prev saves the last command.
function _orgs_snip_widget
  set -l err (mktemp)
  set -l cmd (%[1]s snip print -q (commandline) </dev/tty 2>$err | string collect)
  set -l rc $pipestatus[1]
  if test $rc -eq 0 -a -n "$cmd"
    commandline -r -- $cmd
  else if test $rc -ne 130 -a -s $err
    echo; tail -n 1 $err
  end
  rm -f $err
  commandline -f repaint
end
bind %[3]s _orgs_snip_widget
function snip-prev
  set -l c $history[1]
  if string match -q 'snip-prev*' -- $c
    set c $history[2]
  end
  %[1]s snip new $argv -- $c
end
`

func (self *Snip) shell(words []string) {
	sh := ""
	if len(words) > 0 {
		sh = words[0]
	} else if s := os.Getenv("SHELL"); s != "" {
		sh = s[strings.LastIndex(s, "/")+1:]
	}
	exe := "orgs"
	if p, err := os.Executable(); err == nil {
		exe = p
	}
	exe = "'" + strings.ReplaceAll(exe, "'", `'\''`) + "'"
	key := os.Getenv("ORGS_SNIP_KEY")
	switch sh {
	case "zsh":
		if key == "" {
			key = "^s"
		}
		fmt.Printf(zshScript, exe, key, key)
	case "bash":
		if key == "" {
			key = `\C-s`
		}
		fmt.Printf(bashScript, exe, key, key)
	case "fish":
		if key == "" {
			key = `\cs`
		}
		fmt.Printf(fishScript, exe, key, key)
	default:
		fmt.Fprintln(os.Stderr, "orgs snip shell zsh|bash|fish")
		os.Exit(1)
	}
}

func readAll(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	return string(b), err
}
