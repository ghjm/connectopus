#!/bin/bash

tmux new-session -d -s demo

tmux split -t demo:^
tmux split -t demo:^

tmux select-layout -t demo:^ even-vertical

tmux send-keys -t demo:^.0 \
  'sleep 0.2 && ./connectopus init --id foo --subnet fd00::/8 --ip fd00::1:1 --backend 'type=dtls-dialer,peer=localhost:4444,psk=test-psk' --force --run' \
  Enter

tmux send-keys -t demo:^.1 \
  './connectopus init --id bar --config test.yml --force --run' \
  Enter

tmux send-keys -t demo:^.2 \
  'sleep 0.2 && ./connectopus init --id baz --subnet fd00::/8 --ip fd00::3:1 --backend 'type=dtls-dialer,peer=localhost:4444,psk=test-psk' --force --run' \
  Enter

tmux attach -t demo
