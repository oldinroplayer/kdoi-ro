#!/bin/bash
# Development loop: builds the dev target. If its server is already running,
# switches that server to the new build (players see "restarting" and come
# back by themselves); otherwise starts the server here.
cd "$(dirname "$0")"
pid="$(cat targets/dev/save/server.pid 2>/dev/null)"
if [[ -n "$pid" ]] && [[ "$(ps -o comm= -p "$pid" 2>/dev/null)" == *kdoiro-server ]]; then
  exec ./build.sh dev --update
fi
./build.sh dev && exec targets/dev/run.sh
