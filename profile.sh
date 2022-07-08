#!/bin/bash
make connectopus
mkdir -p pprof
connectopus node --config test.yml --id foo --cpuprofile pprof/foo.pprof --log-level error & pid1="$!"
connectopus node --config test.yml --id bar --cpuprofile pprof/bar.pprof --log-level error & pid2="$!"
connectopus node --config test.yml --id baz --cpuprofile pprof/baz.pprof --log-level error & pid3="$!"
sleep 1
connectopus nsenter --node baz -- iperf3 -s & pid4="$!"
sleep 0.5
iperf3 -c fd00::3:3
kill $pid1 $pid2 $pid3 $pid4
sleep 0.5
go tool pprof -http=: pprof/foo.pprof & pid1="$!"
go tool pprof -http=: pprof/bar.pprof & pid2="$!"
go tool pprof -http=: pprof/baz.pprof & pid3="$!"
trap "kill $pid1 $pid2 $pid3" INT
echo Press enter to terminate...
read
kill $pid1 $pid2 $pid3
