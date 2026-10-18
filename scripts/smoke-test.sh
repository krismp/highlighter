#!/usr/bin/env bash
# Checks an installed desktop app using only what it bundles:
#
#   scripts/smoke-test.sh <Highlighter.app | folder containing Highlighter.exe> [port]
#
# Strict checks: the app starts its server, finds yt-dlp/ffmpeg/deno inside
# itself, a second launch exits and leaves one instance, ffmpeg fetches over
# HTTPS with the bundled CA file and encodes, deno runs, and Quit in the UI
# closes the app. Then it clips a real YouTube video; set YOUTUBE_OPTIONAL=1 to
# only warn when that fails (YouTube often blocks CI runners' datacenter IPs).
set -uo pipefail

app=${1:?usage: scripts/smoke-test.sh <Highlighter.app|install folder> [port]}
port=${2:-7790}
case "$app" in
  *.app)
    main="$app/Contents/MacOS/Highlighter"
    tools="$app/Contents/MacOS" ext="" cacert="$app/Contents/Resources/cacert.pem" ;;
  *)
    main="$app/Highlighter.exe"
    tools="$app" ext=.exe cacert="$app/cacert.pem" ;;
esac
[[ -x $main ]] || { echo "FAIL: no app executable at $main"; exit 1; }

home=$(mktemp -d)
export HOME="$home" USERPROFILE="$home"   # keep settings and clips out of the real profile
export HIGHLIGHTER_PORT=$port
[[ $(uname) == Darwin ]] && export PATH=/usr/bin:/bin   # no Homebrew yt-dlp or ffmpeg
py=$(command -v python3 || command -v python)
tools_native=$(cd "$tools" && pwd)
command -v cygpath >/dev/null && tools_native=$(cygpath -w "$tools_native")   # Git Bash on Windows
api=http://127.0.0.1:$port/api
json='Content-Type: application/json'

logs() {
  cat "$home/Library/Logs/com.github.krismp.highlighter/server.log" 2>/dev/null
  [[ -n ${LOCALAPPDATA:-} ]] && cat "$LOCALAPPDATA/com.github.krismp.highlighter/logs/server.log" 2>/dev/null
}
fail() { echo "FAIL: $*"; logs; curl -s -XPOST -H "$json" "$api/quit" >/dev/null 2>&1; kill "$pid" 2>/dev/null; exit 1; }
field() { "$py" -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
alive() { kill -0 "$1" 2>/dev/null; }

"$main" > "$home/app.log" 2>&1 &
pid=$!
for _ in $(seq 1 120); do curl -sf "$api/ping" >/dev/null && break; alive $pid || break; sleep 1; done
curl -sf "$api/ping" >/dev/null || { cat "$home/app.log"; fail "the app did not start its server"; }
echo "ok: app started its server"

"$main" > "$home/second.log" 2>&1 &
second=$!
for _ in $(seq 1 20); do alive $second || break; sleep 1; done
alive $second && fail "a second launch kept running instead of handing over to the first"
alive $pid && curl -sf "$api/ping" >/dev/null || fail "the first instance stopped after a second launch"
echo "ok: second launch hands over to the running app"

health=$(curl -sf "$api/health") || fail "health"
for tool in ytDlp ffmpeg deno; do
  path=$(echo "$health" | field "d['$tool']['path'] if d['$tool']['found'] else ''")
  [[ -n $path ]] || fail "$tool not found"
  [[ $path == "$tools_native"* ]] || fail "$tool is not the bundled one: $path (want under $tools_native)"
  echo "ok: $tool $(echo "$health" | field "d['$tool']['version']")"
done
encoder=$(echo "$health" | field "d['encoder']")
echo "ok: encoder $encoder"

# The clip pipeline without YouTube: HTTPS through the bundled CA file (static
# ffmpeg builds cannot read the OS trust store), an encode with the chosen
# encoder, and the JavaScript runtime yt-dlp uses for YouTube.
"$tools/ffmpeg$ext" -hide_banner -loglevel error -ca_file "$cacert" -f data \
  -i https://raw.githubusercontent.com/krismp/highlighter/main/README.md -map 0 -c copy -f data - >/dev/null \
  || fail "ffmpeg could not fetch over HTTPS with the bundled CA file"
echo "ok: ffmpeg HTTPS with certificate verification"
"$tools/ffmpeg$ext" -hide_banner -loglevel error -f lavfi -i testsrc=size=640x360:rate=30:duration=2 \
  -f lavfi -i sine=duration=2 -c:v "$encoder" -pix_fmt yuv420p -c:a aac -shortest "$home/encode.mp4" \
  && [[ -s $home/encode.mp4 ]] || fail "ffmpeg could not encode with $encoder"
echo "ok: encoded H.264/AAC MP4 with $encoder"
[[ $("$tools/deno$ext" eval "console.log(6 * 7)") == 42* ]] || fail "deno cannot run JavaScript"
echo "ok: deno runs JavaScript"

clip() {
  local v=jNQXAC9IVRw body='{"leadIn":0,"clips":[{"id":"c1","start":2,"duration":4,"label":"zoo","approved":true}]}'
  local resp
  resp=$(curl -s -XPOST -H "$json" "$api/sessions" -d "{\"url\":\"https://www.youtube.com/watch?v=$v\"}")
  [[ $resp == *'"videoId"'* ]] || { echo "resolve failed: $resp"; return 1; }
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
for _ in $(seq 1 15); do alive $pid || break; sleep 1; done
alive $pid && fail "the app kept running after Quit"
curl -sf --max-time 2 "$api/ping" >/dev/null && fail "the server kept running after Quit"
echo "ok: Quit closed the app and its server"
echo "PASS"
