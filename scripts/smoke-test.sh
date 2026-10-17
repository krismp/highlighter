#!/usr/bin/env bash
# Checks an unpacked release build using only its bundled tools:
#
#   scripts/smoke-test.sh <path to Highlighter.app or Highlighter.exe> [port]
#
# Strict checks: the server starts (the macOS app via its background
# launcher), finds yt-dlp/ffmpeg/deno next to itself, a second launch finds
# the running server, and Quit stops it. Then it clips a real YouTube video;
# set YOUTUBE_OPTIONAL=1 to only warn when that fails (YouTube often blocks
# CI runners' datacenter IPs).
set -uo pipefail

app=${1:?usage: scripts/smoke-test.sh <Highlighter.app|Highlighter.exe> [port]}
port=${2:-7790}
case "$app" in
  *.app) exe="$app/Contents/MacOS/Highlighter" ;;
  *) exe="$app" ;;
esac

home=$(mktemp -d)
export HOME="$home" USERPROFILE="$home"     # keep settings and clips out of the real profile
[[ $(uname) == Darwin ]] && export PATH=/usr/bin:/bin   # no Homebrew yt-dlp or ffmpeg
py=$(command -v python3 || command -v python)
tools_dir=$(cd "$(dirname "$exe")" && pwd)/tools
command -v cygpath >/dev/null && tools_dir=$(cygpath -w "$tools_dir")   # Git Bash on Windows
api=http://127.0.0.1:$port/api
json='Content-Type: application/json'

fail() { echo "FAIL: $*"; curl -s -XPOST -H "$json" "$api/quit" >/dev/null 2>&1; cat "$home/Library/Logs/Highlighter.log" 2>/dev/null; exit 1; }
field() { "$py" -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

if [[ $app == *.app ]]; then
  "$exe" -no-open -port "$port" || fail "launcher exited with $?"
else
  "$exe" -no-open -port "$port" > "$home/server.log" 2>&1 &
fi
for _ in $(seq 1 120); do curl -sf "$api/ping" >/dev/null && break; sleep 1; done
curl -sf "$api/ping" >/dev/null || { cat "$home/server.log" 2>/dev/null; fail "server did not start"; }
echo "ok: server started"

"$exe" -no-open -port "$port" 2>&1 | grep -q "already running" || fail "second launch did not find the running server"
echo "ok: second launch reuses the running server"

health=$(curl -sf "$api/health") || fail "health"
for tool in ytDlp ffmpeg deno; do
  path=$(echo "$health" | field "d['$tool']['path'] if d['$tool']['found'] else ''")
  [[ -n $path ]] || fail "$tool not found"
  [[ $path == "$tools_dir"* ]] || fail "$tool is not the bundled one: $path (want under $tools_dir)"
  echo "ok: $tool $(echo "$health" | field "d['$tool']['version']")"
done
echo "ok: encoder $(echo "$health" | field "d['encoder']")"

clip() {
  local v=jNQXAC9IVRw body='{"leadIn":0,"clips":[{"id":"c1","start":2,"duration":4,"label":"zoo","approved":true}]}'
  curl -sf -XPOST -H "$json" "$api/sessions" -d "{\"url\":\"https://www.youtube.com/watch?v=$v\"}" >/dev/null || { echo "resolve failed"; return 1; }
  curl -sf -XPUT -H "$json" "$api/sessions/$v" -d "$body" >/dev/null && curl -sf -XPUT -H "$json" "$api/sessions/$v" -d "$body" >/dev/null
  curl -sf -XPOST -H "$json" "$api/sessions/$v/process" -d '{"mode":"both","reelName":""}' >/dev/null || { echo "process refused"; return 1; }
  local state=""
  for _ in $(seq 1 90); do
    sleep 2
    state=$(curl -s --max-time 1 "$api/sessions/$v/events" | head -1 | sed 's/^data: //')
    [[ $state == *'"running":false'* ]] && break
  done
  echo "$state" | field "'clip: ' + d['clips'][0]['status'] + ' ' + d['clips'][0]['error'][-300:] + ' | reel: ' + d['concat']['status'] + ' ' + d['concat']['error'][-300:]"
  [[ $state == *'"status":"done"'*'"status":"done"'* ]]
}
if clip; then
  echo "ok: clipped a YouTube video"
elif [[ ${YOUTUBE_OPTIONAL:-} == 1 ]]; then
  echo "WARN: YouTube clip failed (allowed on CI)"
else
  fail "YouTube clip failed"
fi

curl -sf -XPOST -H "$json" "$api/quit" >/dev/null || fail "quit request"
sleep 2
curl -sf --max-time 2 "$api/ping" >/dev/null && fail "server still running after Quit"
echo "ok: Quit stopped the server"
echo "PASS"
