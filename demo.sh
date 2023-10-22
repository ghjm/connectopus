#!/bin/bash
SESSION="connectopus-three-node"
CONNECTOPUS="./connectopus"
CONFIG="./test.yml"
NODES="foo bar baz"

export SESSION CONNECTOPUS CONFIG NODES

if ! ssh-add -l >&/dev/null; then
  echo You must be running an SSH agent with a key loaded to start this demo.
  exit 1
fi

if test -d $HOME/.config/connectopus; then
  echo This demo will overwrite your data directory.
  echo Press Enter to continue or Ctrl+C to exit.
  read
fi

run_demo() {
  for node in $NODES; do
    tmux split-window -v $CONNECTOPUS init --id $node --config $CONFIG --run --force
  done
}

export -f run_demo

tmux new-session -d -s $SESSION run_demo
sleep 0.2
tmux select-layout -t $SESSION even-vertical
tmux attach-session -t $SESSION
