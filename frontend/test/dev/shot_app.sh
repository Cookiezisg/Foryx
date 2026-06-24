#!/usr/bin/env bash
# Real-run screenshot — the REAL Impeller GPU render of the running app (OS-drawn traffic lights, true
# shadows/blur) which the headless Skia path (`make shots`) can't reproduce. Builds the gallery as a
# macOS .app, launches it, resolves its window rect via System Events (not a fixed guess), captures
# that rect with screencapture → test/dev/out/app.png, then quits the app.
#   BUILD=0 reuses an already-built anselm.app (fast); default builds fresh (correct for a final check).
#   TARGET overrides the entrypoint (default the gallery; pass lib/main.dart for the real shell).
# 真跑终检:截运行中 app 的 Impeller 真渲染(红绿灯/真阴影,headless 还原不了)。免前台 sleep——等窗口用
# osascript 内的 delay 轮询。BUILD=0 复用已构建 app(快);默认重构(终检要新)。
set -euo pipefail
cd "$(dirname "$0")/../.."   # frontend/
RUN="mise exec --"
TARGET="${TARGET:-lib/dev/gallery_main.dart}"
APP="build/macos/Build/Products/Debug/anselm.app"
OUT="test/dev/out"; mkdir -p "$OUT"

if [ "${BUILD:-1}" = "1" ] || [ ! -d "$APP" ]; then
  echo "→ flutter build macos --debug -t $TARGET …"
  $RUN flutter build macos --debug -t "$TARGET"
fi

open "$APP"

# Wait for the window and read its rect (position + size, in points). delay lives inside osascript,
# not a foreground shell sleep. 等窗口并读其矩形(点)。
BOUNDS=$(osascript <<'OSA'
set appName to "anselm"
repeat 60 times
  tell application "System Events"
    if (exists (process appName)) then
      tell process appName
        if (count of windows) > 0 then
          set p to position of window 1
          set s to size of window 1
          return ((item 1 of p) as text) & "," & ((item 2 of p) as text) & "," & ((item 1 of s) as text) & "," & ((item 2 of s) as text)
        end if
      end tell
    end if
  end tell
  delay 0.5
end repeat
return "TIMEOUT"
OSA
)
if [ "$BOUNDS" = "TIMEOUT" ]; then echo "✗ anselm window never appeared"; exit 1; fi

osascript -e 'tell application "anselm" to activate'
osascript -e 'delay 0.6'   # let the window settle frontmost before grabbing 让窗口稳定到最前
screencapture -R "$BOUNDS" -o "$OUT/app.png"
osascript -e 'tell application "anselm" to quit' >/dev/null 2>&1 || true
echo "✓ $OUT/app.png  (window rect $BOUNDS)"
