set -o vi
alias gac='git add . && git commit'
alias v='nvim'
alias s='ls'
alias c='clear'
alias e='exit'
alias o='xdg-open'
alias cp='cp -r'
alias vc='nvim /nvim'

export VISUAL=nvim
export EDITOR=nvim

export PATH=/opt/6106/opencilk/bin:$PATH
export PATH=/nvim/build/bin:$PATH
export PATH=$HOME/.local/bin:$PATH
export PATH=$HOME/go/bin:$PATH
if test -n "$JAVA_HOME"; then
  export PATH=$JAVA_HOME/bin:$PATH
fi

# Last, so a course toolchain venv outranks everything above. /nvim ships its
# own python, and without this it shadows the venv: the prompt says the venv
# is active while `import cocotb` fails. Unset outside 6.205, so a no-op.
if test -n "$VIRTUAL_ENV"; then
  export PATH=$VIRTUAL_ENV/bin:$PATH
fi
