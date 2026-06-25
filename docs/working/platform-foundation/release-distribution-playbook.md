---
id: WRK-043
type: working
status: active
owner: @weilin
created: 2026-06-26
reviewed: 2026-06-26
review-due: 2026-09-24
audience: [human, ai]
---

# WRK-043 — 发行与分发 Playbook(三平台,可照敲)

> **一句话**:把 Anselm 从 `flutter build` 一路送到**已签名 / 已公证 / 可安装 / 能自动更新**的三平台发行物的**可执行操作手册**——确切命令、工具、花多少钱、多少时间、CI 怎么搭。是 [[platform-foundation-research]](WRK-042)里"发行"那块(用户痛点)的深挖,且并入了 WRK-042 §5 #8.4/#8.5(商店元数据 + 法务许可)。源:workflow `wh2q3i0pj`(17 agent / ~96 万 token,8 节 + 对抗验证 + 发行就绪闸)。本篇为建造前研究存档,落地后提取进 `references/` 并填 `landed-into`。

## 0. 已焊入的决策(用户已拍板)
- **目标 = 可发行给外部用户**(非仅自用)。
- **macOS = 非沙盒 Developer ID + 公证**,$99/yr 已确认会买。**不上 Mac App Store**。
- **要自动更新**(三平台)。
- **托管抗封**:旧 `anselm.host` 被封 → 更新源/下载**首选 GitHub Releases**(不绑单一域名)。
- **provider Key 归 Go 后端**(非发行范畴)。

## 1. ✅ bundle 标识已改(2026-06-26 完成)
原 `host.anselm.anselm`(双重口吃 + 反向 DNS 含被封域名 `anselm.host`)**已统一改为 `website.anselm.app`**(反向 DNS 取用户在用、可掌控的域名 `anselm.website`,与 GitHub Releases 托管无关、抗封)。改动落点:`macos/Runner/Configs/AppInfo.xcconfig`(bundle id + 版权→Anselm)· `macos/Runner.xcodeproj/project.pbxproj`(RunnerTests target ×3)· `linux/CMakeLists.txt`(`APPLICATION_ID`)· `windows/runner/Runner.rc`(CompanyName/版权→Anselm)。`flutter build macos` 验证通过。
> 备注:免费档网关已早搬至 `api.anselm.website`(`backend/internal/infra/llm/anselm.go`),旧 `anselm.host` 仅曾用于免费 key 端点、现无活引用。`anselm://` URL scheme 注册属深链 P0(尚未落地),届时归到此 bundle id。

## 2. 成本与时间(year-1)
| 项 | 成本 | 说明 |
|---|---|---|
| Apple Developer Program | **$99/yr** | 个人 1–3 天批;org 要 D-U-N-S、7–10 天 |
| Windows 代码签名 | **~$120/yr**(Azure Trusted/Artifact Signing,~$9.99/mo,**若符合资格**)否则云 HSM OV 证书 **~$200–300/yr** | ⚠ **EV 证书 2024 起不再绕过 SmartScreen**,别为此多花钱;声誉靠真实下载量数周养成 |
| GitHub Releases(更新源 + 下载托管) | **$0** | 抗封、URL 稳定;见 §自动更新的 raw-asset 注意点 |
| Linux(AppImage + GPG + zsync) | **$0** | 无 gatekeeper、最快出首个签名物 |
| **year-1 合计** | **≈ $220–400** | 主变量是 Windows 证书路径 |
> 首个签名物到手:**Linux 最快**(零审查),建议先打通它建立信心;macOS/Windows 受账户/证书 lead time 限制,**并行尽早开办**。

## 3. 关键路径(最短可发行路径,详见正文末「发行就绪」)
1. **定身份**:改掉 `host.anselm.anselm`(§1)→ 生成各平台图标 → 定版本号方案(系于 `pubspec`)→ 注册 `anselm://`。
2. **并行办证**(按 lead time):Apple($99,1–3 天)· Windows 签名(**现在就开**,SmartScreen 声誉要数周)· Linux GPG(免费即时)。
3. **Linux 先出**:`flutter build linux` → 内嵌对应 arch 的 Go sidecar → AppImage(注意 Ubuntu 24.04 的 FUSE3/libfuse2t64 坑,带 `--appimage-extract-and-run` 兜底)+ `.desktop` + 图标 + AppStream + GPG/SHA256 + zsync 指向 GitHub Releases。
4. macOS / Windows 签名公证(§对应节)→ 自动更新接 GitHub Releases appcast。

## 4. 对抗复审揪出的关键纠正(动手前必看,正文每节末「⚠ 复审纠正」有详情)
- **macOS staple 顺序**:先 staple `.app` 再打/再 staple DMG(不能给未构建的 .app 事后 staple)。
- **签名 inside-out**:先签嵌套 Go sidecar(带自己的 entitlements)再签 .app,**绝不 `--deep` 签**;`-o runtime --timestamp` 必带。
- **macOS entitlements(让"下载来的运行时"不被 SIGKILL)**:`disable-library-validation`(下载的运行时是别的 Team 签的)+ `allow-dyld-environment-variables`(EnvManager 设 PATH/DYLD)+ `allow-jit` + `allow-unsigned-executable-memory`(V8/CPython),放在**真正 spawn 解释器的 sidecar** 上。
- **Windows**:`azure/trusted-signing-action@v0` 已过期(改用当前 action);证书有效期上限 **460 天**(CSC-31,2025-11);**EV 不再绕 SmartScreen**;sidecar **必须绑 127.0.0.1** 否则每个用户弹 Defender 防火墙入站框。
- **Linux**:AppImage 在 Ubuntu 24.04 的 FUSE2→FUSE3 断裂,必须处理。
- **自动更新**:`winsparkle-tool generate-key` 语法(写法有误,见正文);**EdDSA(ed25519)更新密钥**是独立于 Apple/Authenticode 的**第二把私钥**,CI 里单独托管;macOS 更新仍须投递**已公证+staple** 的 .app。
- **法务**:`go-licenses` 安装路径已变;USPTO TESS 已于 2023-11 停用(改 cloud Trademark Search);商标费数字需更新;**下载的运行时**(python/node/uv/dotnet)非捆绑→无再分发义务,但 EULA 须含"执行任意代码 + AI 输出免责"。

## 5. 仍未定 / 后续
- **#5 appcast 托管**:正文推荐 **GitHub Releases**(抗封、$0),但有 raw-asset URL 稳定性注意点(见正文)——等你新域名/托管最终拍板。
- **#8.2 i18n 运行时格式化 + 时区/DST**(尤其 Scheduler cron/夏令时正确性):仍是缺口,建议晚点单独一小轮,不阻塞发行。

## 6. 数据/复核
- 结构化:`scratchpad/wf3_digest.json`(8 节 + 复审 + 完整性闸)。原始全文(含全部命令 + 引用):`tasks/wh2q3i0pj.output`。
- 下方正文 = 8 节逐节(目标 / 前置含成本 / 步骤含命令 / 与我们 / 工具 / 坑 / 成本时间 / ⚠ 复审纠正)+ 发行就绪关键路径 + 仍缺项。
## macos-sign-notarize

> **目标**:Take a release-built, non-sandboxed Flutter `.app` (that bundles a nested Go sidecar `cmd/server` and will later download+exec python/node/uv/dotnet) all the way to a Developer-ID-signed, Hardened-Runtime, notarized, stapled `.dmg` that opens cleanly on any external user's Mac (Intel + Apple Silicon) with no Gatekeeper warning, and whose downloaded runtimes also run without being SIGKILLed.

**前置(含成本)**
- **Apple Developer Program membership (individual or org)** — _$99/yr USD, recurring annual_ — Enroll at https://developer.apple.com/programs/enroll/ with an Apple Account that has 2FA on. Individual enrollment is near-instant; org enrollment requires a D-U-N-S number and can take days. This is the gate for getting a Developer ID Application cert and fo
- **Developer ID Application signing certificate + private key** — _included in the $99/yr; no extra cost_ — In Xcode: Settings > Accounts > (your team) > Manage Certificates > + > 'Developer ID Application'. Or via developer.apple.com > Certificates > + > 'Developer ID Application'. This creates the cert in your login keychain. Verify with `security find-identity -v
- **App Store Connect API key (Team key, Developer role) for notarytool** — _included in the $99/yr_ — App Store Connect > Users and Access > Integrations > App Store Connect API > Team Keys > generate a key with at minimum 'Developer' access (personal keys are NOT accepted by the Notary service). Download the AuthKey_XXXX.p8 ONCE (cannot re-download), and reco
- **A Mac with current Xcode command-line tools (codesign, notarytool, stapler, spctl)** — _free (hardware aside)_ — `xcode-select --install` or full Xcode. notarytool ships with Xcode 13+. Confirm: `xcrun notarytool --version` and `xcrun stapler --help`. Signing+notarization MUST happen on macOS (no cross-host substitute), so the release job for the mac artifact runs on a M
- **create-dmg (DMG builder) — pick ONE vehicle** — _free, MIT_ — `brew install create-dmg` (the create-dmg/create-dmg shell tool, actively maintained). Alternative: the `dmg` pub package (lamnhan066) wraps build+sign+notarize+staple but is less transparent. fastforge can also emit a DMG. Recommend create-dmg for control: it

**步骤**
1. **Build the release .app and stage the freshly-built Go sidecar inside it. Build the sidecar per target arch (pure-Go, no CGO per ADR 0001) and copy it into Contents/MacOS (or Contents/Resources) of the .app. For a universal app, lipo-merge the arm64+amd64 sidecar binaries.**
   ```
   # sidecar, both arches, then universal
   GOOS=darwin GOARCH=arm64 go build -o build/server-arm64 ./cmd/server
   GOOS=darwin GOARCH=amd64 go build -o build/server-amd64 ./cmd/server
   lipo -create -output build/server build/server-arm64 build/server-amd64
   flutter build macos --release
   # copy sidecar into the bundle (a Copy Files build phase is the durable way; manual shown for clarity)
   cp build/server build/macos/Build/Products/Release/Anselm.app/Contents/MacOS/server
   chmod +x build/macos/Build/Products/Release/Anselm.app/Contents/MacOS/server
   ```
   — Bundling the sidecar inside the .app is what makes it update in lockstep with the app (one notarized artifact). Do NOT ship it as a separate download. The exec bit must be set.
2. **Author TWO entitlements files. The PARENT app (and the sidecar, which is the process that actually spawns/exec's the interpreters) need the Hardened-Runtime relaxations; child interpreters do NOT inherit them so the relaxations must live on whichever Mach-O actually does the JIT/exec. Keep App Sandbox OFF (do not add com.apple.security.app-sandbox).**
   ```
   <!-- macos/Runner/Release.entitlements (the Flutter app) -->
   <key>com.apple.security.cs.allow-jit</key><true/>
   <key>com.apple.security.cs.allow-unsigned-executable-memory</key><true/>
   <key>com.apple.security.cs.disable-library-validation</key><true/>
   <key>com.apple.security.cs.allow-dyld-environment-variables</key><true/>
   <!-- sidecar.entitlements (the Go cmd/server, which spawns python/node) -->
   <key>com.apple.security.cs.disable-library-validation</key><true/>
   <key>com.apple.security.cs.allow-dyld-environment-variables</key><true/>
   <key>com.apple.security.cs.allow-jit</key><true/>
   <key>com.apple.security.cs.allow-unsigned-executable-memory</key><true/>
   ```
   — WHY each: disable-library-validation — REQUIRED, because the downloaded runtimes are signed by Astral/nodejs.org/Microsoft (a DIFFERENT Team ID than yours) and load their own dylibs; without it, loading third-party-signed libs into a hardened process is blocked. allow-dyld-environment-variables — REQUIRED, because EnvManager sets PATH/DYLD_/venv env when spawning runtimes. allow-jit + allow-unsign
3. **Sign INSIDE-OUT (bottom-up), never --deep. Apple's Quinn explicitly flags --deep as harmful: it applies one entitlement set to everything and can break trusted execution. Sign the nested Go sidecar (and any other nested Mach-O / frameworks) FIRST with its own entitlements, then the app's own embedded frameworks, then the main app executable, then the .app last.**
   ```
   ID="Developer ID Application: Your Name (TEAMID)"
   APP=build/macos/Build/Products/Release/Anselm.app
   # 1) nested sidecar first, with the sidecar entitlements
   codesign -f -s "$ID" -o runtime --timestamp --entitlements sidecar.entitlements "$APP/Contents/MacOS/server"
   # 2) any embedded frameworks / helper dylibs (example)
   find "$APP/Contents/Frameworks" -name '*.framework' -o -name '*.dylib' | while read f; do \
     codesign -f -s "$ID" -o runtime --timestamp "$f"; done
   # 3) the main app last, with the app entitlements
   codesign -f -s "$ID" -o runtime --timestamp --entitlements macos/Runner/Release.entitlements "$APP"
   # verify the whole tree (must say 'valid on disk' / 'satisfies its Designated Requirement')
   codesign -vvv --deep --strict "$APP"
   spctl -a -vvv -t exec "$APP"  # pre-notarization this may say 'rejected (no usable signature)'; that's expected until stapled
   ```
   — -o runtime turns on Hardened Runtime (mandatory for notarization). --timestamp embeds a secure timestamp (mandatory — notarization rejects un-timestamped signatures). -f forces re-sign. codesign --deep is fine for VERIFYING (read-only) but never for signing. The sidecar gets sidecar.entitlements, the app gets Release.entitlements — different sets, which is exactly why --deep can't be used.
4. **Store notarytool credentials once using the App Store Connect API .p8 key. This writes a named profile into the keychain so later submits don't need the raw secrets on the command line.**
   ```
   xcrun notarytool store-credentials "anselm-notary" \
     --key ~/private_keys/AuthKey_XXXXXXXXXX.p8 \
     --key-id XXXXXXXXXX \
     --issuer xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   ```
   — The profile name 'anselm-notary' is referenced later as --keychain-profile. Personal API keys are rejected — must be a Team key with Developer role. In CI, either pre-seed this profile or pass --key/--key-id/--issuer inline from secrets.
5. **Build the DMG from the signed .app (create-dmg), then SIGN the DMG itself with the same Developer ID. The DMG container is signed separately from its contents.**
   ```
   create-dmg \
     --volname "Anselm" \
     --app-drop-link 600 185 \
     --window-size 800 400 \
     Anselm.dmg "$APP"
   codesign -f -s "$ID" --timestamp Anselm.dmg
   ```
   — The .app inside was already signed in step 3; create-dmg just packages it. Signing the DMG itself gives the download container a verifiable signature. (No -o runtime needed on the DMG — Hardened Runtime is an executable concept, not a disk image one.)
6. **Submit the DMG to Apple's Notary service and wait for the result. Notary scans for malware and checks that everything is Developer-ID-signed with Hardened Runtime + secure timestamp.**
   ```
   xcrun notarytool submit Anselm.dmg --keychain-profile "anselm-notary" --wait
   # if it comes back 'Invalid', pull the detailed log:
   # xcrun notarytool log <submission-id> --keychain-profile "anselm-notary"
   ```
   — --wait blocks until Apple returns Accepted/Invalid (typically 1–5 min, occasionally longer). Common Invalid causes here: an unsigned nested Mach-O (the sidecar was missed), a signature without --timestamp, or Hardened Runtime not enabled. The `notarytool log` JSON pinpoints the exact path. You can notarize the .app (zipped via `ditto -c -k --keepParent`) instead of/in addition to the DMG, but nota
7. **Staple the notarization ticket onto the DMG so Gatekeeper can verify OFFLINE (the user's Mac doesn't have to phone Apple at open time). Then verify the final artifact passes Gatekeeper.**
   ```
   xcrun stapler staple Anselm.dmg
   xcrun stapler validate Anselm.dmg
   spctl -a -vvv -t open --context context:primary-signature Anselm.dmg  # should say 'accepted, source=Notarized Developer ID'
   ```
   — Stapling is what makes a first-run on a fresh machine (or air-gapped) clean. The ticket is also retrievable online, but stapling avoids the spinner / failure when offline. The .app extracted from a stapled DMG carries its own stapled ticket too (staple the .app before DMG-building if you also distribute a raw zip).
8. **Handle the DOWNLOADED runtimes (python/node/uv/dotnet) so they are NOT killed. These arrive AFTER install via directInstaller (ADR 0001) and are NOT inside your signed bundle, so notarization does not cover them. Two independent macOS mechanisms apply: (a) Gatekeeper/quarantine — neutralized by stripping the quarantine xattr after extraction (ADR 0001 already does `xattr -cr`); (b) the Apple-Silicon mandatory-signature kernel check — satisfied because the upstream runtimes ship publisher-signed (Astral/nodejs.org/Microsoft) Mach-O, NOT linker-only.**
   ```
   # directInstaller already does, post-extract, in the runtime dir:
   xattr -cr "<sandboxRoot>/runtimes/<kind>/<version>"
   # DEFENSIVE belt-and-suspenders for any binary lacking even an ad-hoc sig on arm64
   # (only needed if a future runtime ships linker-only / unsigned):
   # codesign -f -s - "<path-to-that-binary>"   # ad-hoc re-sign in place
   ```
   — WHY this works: quarantine (com.apple.quarantine xattr) is what triggers Gatekeeper's full notarization assessment on a downloaded file; clearing it (xattr -cr) drops the binary to the lighter code-signing check. On Apple Silicon EVERY executable still needs at least a valid (even ad-hoc) signature or it is SIGKILLed with 'Killed: 9' / 'load code signature error'. The pinned upstream tarballs are 

**与我们(sidecar/下载运行时)**:Two of our architecture facts dominate this section. (1) BUNDLED Go sidecar: cmd/server ships inside Contents/MacOS of the .app. It is a nested Mach-O, so it MUST be signed bottom-up FIRST (step 3), with -o runtime + --timestamp, BEFORE the app, and it must NOT be reached via --deep (it needs its own entitlement file distinct from the app's). It is the process that actually spawns python/node, so the JIT/exec/dyld/library-validation entitlements belong on the sidecar (not only the Flutter app) — child processes do NOT inherit entitlements. Because the sidecar lives in the notarized bundle, it updates in lockstep with the app: one DMG, one notarization, no separate sidecar update channel. (2)

**工具**:`Apple Developer Program`（macOS (signing/notarizat） · `codesign`（macOS） · `xcrun notarytool`（macOS） · `xcrun stapler`（macOS） · `spctl`（macOS） · `create-dmg`（macOS） · `Hardened Runtime entitlements reference`（macOS）
**坑**:1) --deep is a trap: it's the easy one-liner everyone reaches for, but it applies ONE entitlement set to the nested sidecar AND the app and can silently break trusted execution — sign inside-out instead. 2) Child processes do NOT inherit entitlements: putting allow-jit only on the Flutter .app does nothing for node/python spawned by the Go sidecar; the relaxations must be on the SIDECAR (the actual spawner). 3) The downloaded runtimes are NOT notarized and never will be — they rely on (a) xattr 
**成本/时间**:$99/yr USD (Apple Developer Program) — the only paid prerequisite. All tooling (codesign, notarytool, stapler, spctl, cr ｜ First-time setup: ~0.5–1 day (enrollment can be instant for individuals or several days for orgs needing a D-U-N-S numbe

> ⚠ **对抗复审纠正** — **命令**:Mostly correct and current, with a few real issues. (1) STAPLING ORDER FLAW — step 7 staples only the DMG. You cannot staple an unbuilt .app retroactively; the correct, Apple-recommended order is: zip the .app (ditto -c -k --keepParent) → notarize → `stapler staple Anselm.app` → build DMG from the stapled app → notariz ｜ **成本**:Accurate. $99/yr USD for the Apple Developer Program (individual or org) is confirmed current for 2025-2026 from Apple's own membership page. No per-notarization fee — correct (Notary service is free, included). All tooling (codesign, notarytool, stapler, spctl, create-dmg) is free — correct. One addition worth noting  ｜ **我们特殊点**:Largely holds, but contains ONE technically-wrong causal claim that should be corrected even though the end result still works. THE BUNDLED SIDECAR handling is correct: nested Mach-O must be signed bottom-up first with its own entitlements + -o runtime + --timestamp before the app, never via --deep, and it updates in l ｜ **工具纠正**:`codesign`→--deep-considered-harmful guidance (Quinn, forums/thread/129980) verified accurate; inside；`xcrun notarytool`→Personal API keys rejected / Team key with Developer role required is VERIFIED accurate. A；`xcrun stapler`→see stapling-order flaw: staple the .app before DMG packaging, not only the DMG

---

## Auto-update end-to-end across macOS/Windows/Linux with ban-resistant hosting (GitHub Releases appcast)

> **目标**:From a packaged build to a signed, verified, auto-updating artifact on all three OSes: macOS+Windows via auto_updater (Sparkle/WinSparkle) reading a static appcast.xml, Linux via AppImageUpdate/zsync — all served off GitHub Releases (raw asset URLs) so no single custom domain can be banned. The whole app bundle (Flutter exe + nested Go sidecar + runtime metadata) is replaced atomically per update, so the sidecar travels in lockstep automatically. EdDSA(ed25519)-signed updates on macOS/Windows; the macOS .app stays Developer-ID-signed + notarized + Hardened-Runtime through the swap.

**前置(含成本)**
- **auto_updater Flutter package v1.0.0 (Sparkle macOS + WinSparkle Windows; NO Linux)** — _$0 — MIT, pub.dev_ — flutter pub add auto_updater (pins Sparkle 2.x via CocoaPods on macOS, bundles WinSparkle DLL on Windows). Verified publisher leanflutter.dev.
- **Sparkle tools (generate_keys, generate_appcast, sign_update) for the macOS feed** — _$0 — open source_ — Download Sparkle 2.x distribution (sparkle-project.org) or `brew install --cask sparkle`; the bin/ folder ships generate_keys / generate_appcast / sign_update. CocoaPods pod 'Sparkle' also vendors them under the pod's bin/.
- **WinSparkle + winsparkle-tool for the Windows feed** — _$0 — open source (vslavik/winsparkle)_ — auto_updater bundles WinSparkle. Grab the WinSparkle release zip / NuGet for bin/winsparkle-tool.exe (generates EdDSA keys + signs updates).
- **AppImageUpdate / appimageupdatetool + zsync (Linux only)** — _$0 — open source (AppImageCommunity/AppImageUpdate)_ — Download appimageupdatetool-x86_64.AppImage from github.com/AppImageCommunity/AppImageUpdate releases; install zsync (`apt install zsync`) on the build box so appimagetool can emit the .zsync file.
- **Apple Developer Program (already a prereq for the macOS signing section)** — _$99/yr_ — developer.apple.com — Developer ID Application cert. Needed because every Sparkle bundle swap must land a fully notarized+stapled .app.
- **GitHub repo with Releases enabled (the ban-resistant update host)** — _$0 (public repo) — free egress_ — Any GitHub repo. Release assets get stable URLs https://github.com/<org>/<repo>/releases/download/<tag>/<asset>; the appcast.xml is itself a release asset (or committed file served via raw.githubusercontent.com).

**步骤**
1. **Generate the macOS EdDSA (ed25519) key pair ONCE. Private key lands in the login Keychain (never in git); the printed public key goes into Info.plist as SUPublicEDKey.**
   ```
   ./bin/generate_keys
   # prints:
   #   <key>SUPublicEDKey</key>
   #   <string>pfIShU4dEXqPd5ObYNfDBiQWcXozk7estwzTnF9BamQ=</string>
   # re-run anytime to re-print the public key; export for CI with:
   ./bin/generate_keys -x sparkle_private_key.pem   # keep OUT of git, store as a GitHub Actions secret
   ```
   — Keychain storage means the signing machine/CI runner holds the private key. For CI, export once with -x and stash as an encrypted secret; import on the runner before generate_appcast.
2. **Generate the Windows EdDSA key pair with winsparkle-tool; put the public key in Runner.rc, keep private.key as a CI secret.**
   ```
   winsparkle-tool.exe generate-key > winsparkle.key   # produces ed25519 private key
   # Add public key to windows/runner/Runner.rc:
   #   EdDSAPub  EDDSA  {"pXAx0wfi8kGbeQln11+V4R3tCepSuLXeo7LkOeudc/U="}
   ```
   — WinSparkle uses the SAME EdDSA/ed25519 scheme as Sparkle (DSA is legacy — do NOT use it). Public key embedded as an RC resource, not Info.plist.
3. **Wire the Flutter client. setFeedURL to the GitHub-hosted appcast, set a 24h scheduled check, expose a manual 'Check for Updates'.**
   ```
   final updater = AutoUpdater();
   await updater.setFeedURL(
     'https://github.com/anselm-org/anselm/releases/latest/download/appcast-macos.xml', // win: appcast-windows.xml
   );
   await updater.setScheduledCheckInterval(86400); // 24h; min 3600, 0=off
   // manual menu item -> updater.checkForUpdates();
   ```
   — Use /releases/latest/download/<asset> — GitHub redirects it to the newest release, so the feed URL is STABLE across versions. Pick the per-OS appcast at runtime (Platform.isMacOS ? macos : windows).
4. **Add SUFeedURL + SUPublicEDKey to macOS Info.plist (belt-and-suspenders alongside setFeedURL).**
   ```
   <!-- macos/Runner/Info.plist -->
   <key>SUFeedURL</key>
   <string>https://github.com/anselm-org/anselm/releases/latest/download/appcast-macos.xml</string>
   <key>SUPublicEDKey</key>
   <string>pfIShU4dEXqPd5ObYNfDBiQWcXozk7estwzTnF9BamQ=</string>
   <key>SUEnableInstallerLauncherService</key><false/>
   ```
   — Sparkle compares CFBundleVersion (build number), NOT the marketing string — bump pubspec version: x.y.z+N every release or Sparkle sees 'no update'.
5. **Build + package the full bundle for the release (Flutter exe + nested Go sidecar), sign bottom-up, notarize, staple. This is the artifact the appcast will point at.**
   ```
   # sidecar first (ADR 0001 plain cross-compile), then app:
   (cd backend && GOOS=darwin GOARCH=arm64 go build -o ../frontend/macos/sidecar/anselm-server ./cmd/server)
   flutter build macos --release
   # sign sidecar BEFORE the .app (inside-out, NEVER --deep):
   codesign -f -o runtime --timestamp \
     --entitlements macos/sidecar.entitlements \
     -s 'Developer ID Application: <you>' \
     build/macos/Build/Products/Release/Anselm.app/Contents/MacOS/anselm-server
   codesign -f -o runtime --timestamp \
     --entitlements macos/Runner/Release.entitlements \
     -s 'Developer ID Application: <you>' \
     build/macos/Build/Products/Release/Anselm.app
   ditto -c -k --keepParent build/.../Anselm.app Anselm-1.2.0.zip
   xcrun notarytool submit Anselm-1.2.0.zip --keychain-profile ANSELM_NOTARY --wait
   xcrun stapler staple build/.../Anselm.app
   ```
   — The nested anselm-server is signed with its OWN entitlements (allow-jit / disable-library-validation / allow-unsigned-executable-memory / allow-dyld-environment-variables per the packaging section). Sparkle ALSO signs its own XPC helpers — preserve their signatures, hence no --deep. Re-zip the STAPLED .app for distribution.
6. **Generate the macOS appcast with enclosures pointing at GitHub Releases via --download-url-prefix. generate_appcast auto-signs each enclosure with the Keychain EdDSA key and emits .delta files.**
   ```
   ./bin/generate_appcast \
     --download-url-prefix 'https://github.com/anselm-org/anselm/releases/download/v1.2.0/' \
     --channel stable \
     -o ./dist/appcast-macos.xml \
     ./dist/updates/        # folder holding Anselm-1.2.0.zip (+ older zips for delta gen)
   ```
   — --download-url-prefix decouples 'where the appcast lives' from 'where the binaries live' — appcast + zips both become GitHub release assets but the prefix makes the enclosure URLs absolute GitHub URLs. --channel stable|beta partitions feeds (clients on beta see both).
7. **Generate/sign the Windows appcast. Build+sign the .exe installer (Authenticode, per the Windows section), then sign the enclosure with winsparkle-tool and hand-edit appcast-windows.xml enclosure URLs to the GitHub release.**
   ```
   (cd backend && GOOS=windows GOARCH=amd64 go build -o ../frontend/windows/sidecar/anselm-server.exe ./cmd/server)
   flutter build windows --release
   # package via fastforge -> AnselmSetup-1.2.0.exe, Authenticode-sign it, then:
   winsparkle-tool.exe sign -f winsparkle.key AnselmSetup-1.2.0.exe
   # -> paste stdout as sparkle:edSignature in appcast-windows.xml enclosure
   ```
   — WinSparkle has no generate_appcast equivalent — author appcast-windows.xml by hand (template below) and run winsparkle-tool sign per asset. enclosure url = the GitHub release download URL of the signed installer.
8. **Author the appcast.xml (macOS template shown; Windows identical minus delta). Mark mandatory builds with sparkle:criticalUpdate and gate min-OS with sparkle:minimumSystemVersion.**
   ```
   <rss xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
    <channel>
     <item>
      <title>1.2.0</title>
      <sparkle:channel>stable</sparkle:channel>
      <sparkle:version>120</sparkle:version>            <!-- CFBundleVersion -->
      <sparkle:shortVersionString>1.2.0</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>12.0</sparkle:minimumSystemVersion>
      <sparkle:criticalUpdate sparkle:version="118"/>   <!-- force-update anyone below 118 -->
      <enclosure
        url="https://github.com/anselm-org/anselm/releases/download/v1.2.0/Anselm-1.2.0.zip"
        sparkle:edSignature="7cLALFUHSwvE...gq6mGkt2RBw=="
        length="48213004"
        type="application/octet-stream"/>
     </item>
    </channel>
   </rss>
   ```
   — sparkle:criticalUpdate makes Sparkle refuse to skip / harder-prompt. Omit it for optional updates. minimumSystemVersion lets you drop old-OS users without breaking them.
9. **Publish: cut a GitHub Release tagged v1.2.0 and upload BOTH appcasts + all platform artifacts as release assets. This is the entire 'server'.**
   ```
   gh release create v1.2.0 \
     ./dist/Anselm-1.2.0.zip \
     ./dist/Anselm-1.2.0.zip.delta \
     ./dist/AnselmSetup-1.2.0.exe \
     ./dist/Anselm-1.2.0-x86_64.AppImage \
     ./dist/Anselm-1.2.0-x86_64.AppImage.zsync \
     ./dist/appcast-macos.xml \
     ./dist/appcast-windows.xml \
     --title 'Anselm 1.2.0' --notes-file CHANGELOG-1.2.0.md
   ```
   — Ban-resistance: the feed lives on github.com (not a custom domain). If GitHub itself were ever the problem, the same assets re-host on any S3/R2/CDN and you flip setFeedURL — keep the EdDSA keys constant so old clients still verify. Mirror to a second release repo for redundancy if desired.
10. **Linux (auto_updater has NO Linux): embed gh-releases-zsync update info into the AppImage at build time; the app shells out to appimageupdatetool to self-update.**
   ```
   # build with update info baked in:
   VERSION=1.2.0 ARCH=x86_64 \
   appimagetool -u 'gh-releases-zsync|anselm-org|anselm|latest|Anselm-*-x86_64.AppImage.zsync' \
     Anselm.AppDir Anselm-1.2.0-x86_64.AppImage   # zsync installed -> emits the .zsync too
   # in-app 'check/apply update' (background, then prompt+relaunch):
   appimageupdatetool --check-for-update "$APPIMAGE"   # exit 1 = update available
   appimageupdatetool "$APPIMAGE"                      # delta-downloads new AppImage in place
   ```
   — gh-releases-zsync|<user>|<repo>|latest|<glob> resolves the newest release automatically (same ban-resistant GitHub host). zsync = delta download (only changed squashfs blocks). New AppImage contains the new sidecar -> lockstep holds. No EdDSA here; rely on HTTPS + GitHub provenance (optionally GPG-sign the AppImage).
11. **Update UX + sidecar-aware restart: before applying, ask the running sidecar for in-flight durable runs; defer or warn; on relaunch the swapped bundle re-spawns the new sidecar.**
   ```
   // pseudo: gate install on a safe moment
   final busy = await dio.get('$base/api/v1/flowruns?status=running');
   if (busy.data['data'].isNotEmpty) { showDeferUpdateDialog(); return; }
   // Sparkle/WinSparkle handle download+relaunch; our ProcessManager kills the
   // old sidecar (SIGTERM->grace->SIGKILL) on quit, relaunch spawns the NEW one.
   ```
   — Because Sparkle/AppImageUpdate replace the WHOLE bundle atomically, the nested Go sidecar is updated for free — no separate sidecar update channel exists or is needed. The only client work is: don't kill an in-flight run mid-update, and ensure the post-relaunch health-gate points at the new sidecar.

**与我们(sidecar/下载运行时)**:BUNDLED GO SIDECAR — lockstep is automatic, not a feature you build. Sparkle (full-bundle .zip swap) and AppImageUpdate (whole-AppImage zsync swap) replace the ENTIRE artifact; the nested cmd/server binary lives inside (.app/Contents/MacOS/anselm-server, AppDir, windows/sidecar/) so it travels along. There is deliberately NO separate sidecar update feed (Shorebird/Dart-only OTA is rejected for exactly this reason — it can't move native binaries and would desync the client/contract from the sidecar, a correctness bug per ADR 0004). REQUIREMENTS this imposes: (1) the nested sidecar must be Developer-ID-signed + notarized as part of the bundle (step 5, inside-out, no --deep) or the swapped .app

**工具**:`auto_updater (Flutter)`（macOS (Sparkle), Windows） · `Sparkle 2.x (generate_keys / generate_appcast / sign_update)`（macOS） · `WinSparkle + winsparkle-tool`（Windows） · `AppImageUpdate / appimageupdatetool + appimagetool + zsync`（Linux） · `GitHub Releases (update host)`（All three (host for appc） · `fastforge (packaging umbrella, upstream of this section)`（All three）
**坑**:1) Sparkle compares CFBundleVersion (build NUMBER), not the marketing string — if you bump 1.1.0->1.2.0 but leave +1, Sparkle sees 'no update'. Always bump pubspec version: x.y.z+N. 2) NEVER codesign --deep — it clobbers Sparkle's XPC helper (Autoupdate.app) and WinSparkle signatures; sign inside-out (sidecar -> Sparkle helpers -> .app), or notarization rejects with 'Hardened Runtime disabled in Autoupdate.app' (Sparkle issue #1389/#1641). 3) auto_updater is macOS+Windows ONLY — Linux MUST go th
**成本/时间**:$99/yr (Apple Developer Program, shared with the macOS signing section — required so each Sparkle bundle swap stays nota ｜ First-time setup: ~2-3 days — generate+vault EdDSA keys (both OSes, ~1h), wire auto_updater + Info.plist/Runner.rc (~hal

> ⚠ **对抗复审纠正** — **命令**:Mostly correct, with command-syntax errors:

1. STEP 2 winsparkle-tool is WRONG. Playbook shows `winsparkle-tool.exe generate-key > winsparkle.key`. Real syntax (verified vs vslavik/winsparkle README): `winsparkle-tool generate-key --file private.key` — the PRIVATE key is written to the file via `--file`, and the PUBLI ｜ **成本**:Accurate for 2025-2026. Apple Developer Program $99/yr is current and correctly noted as SHARED with the macOS signing section (not new cost). All update tooling (auto_updater/Sparkle/WinSparkle/AppImageUpdate/zsync/fastforge) genuinely $0 MIT/open-source. GitHub Releases hosting + egress free on public repos — correct ｜ **我们特殊点**:Core architecture is SOUND and reasoning correct, with real flaws to harden:

HOLDS UP:
- Lockstep-via-whole-bundle-swap is genuinely automatic. Sparkle swaps the entire .app (.zip), AppImageUpdate swaps the whole AppImage (zsync); the nested anselm-server at .app/Contents/MacOS/ travels along for free. Rejecting Shore ｜ **工具纠正**:`auto_updater (Flutter)`→Accurate. Low cadence flagged correctly. Nuance: it's a thin wrapper over a PINNED Sparkle；`Sparkle 2.x (generate_keys/gen`→Accurate. CFBundleVersion (build number, not marketing string) comparison correct and a re；`WinSparkle + winsparkle-tool`→Tool real, but generate-key COMMAND in step 2 is wrong (stdout redirect instead of --file)

---

## Costs, Accounts & Timeline to First Signed Build (tri-platform: macOS + Windows + Linux)

> **目标**:A concrete "what do I actually buy, in what order, and how long until each platform has its first SIGNED, trusted, installable artifact" answer for Anselm — covering every account/cert/service, 2025-2026 dollar costs, the eligibility traps that block the obvious cheap path, the buy-order that unblocks the most, realistic time-to-first-signed-build per OS for a first-timer, and what is safe to defer. This section is the budget + procurement + scheduling spine the other release sections build on.

**前置(含成本)**
- **Apple Developer Program membership (individual)** — _$99/yr recurring (USD; varies by region). Notarization via n_ — Enroll at developer.apple.com/programs/enroll with an Apple ID + 2FA + a government legal name match. Individual/sole-proprietor: pay at enrollment. Approval normally 24-48h but in early 2026 many report 2-7+ weeks with no comms — treat as a long-lead item and
- **Windows code-signing certificate (cloud OV — NOT Azure Trusted Signing for a non-US/CA individual)** — _Certum Open Source Code Signing on SimplySign cloud ~$58-69 _ — Certum: certum.store -> Open Source Code Signing on SimplySign; identity verification via government ID + utility bill + active OSS project URL; install SimplySign mobile + desktop apps (virtual cloud HSM, no physical token). SSL.com: ssl.com IV/OV -> validate
- **Update-feed + artifact hosting (ban-resistant, NOT a custom domain)** — _$0. GitHub Releases (binaries + appcast.xml/update-index as _ — Use the existing GitHub repo's Releases as the canonical host: upload signed DMG/.exe/AppImage + appcast.xml (Sparkle/WinSparkle) or update-index.json (desktop_updater) per release via `gh release create`. This is portable and ban-resistant — the prior anselm.
- **Build machines (no purchase if you own them)** — _$0 if you already have the hardware. macOS signing+notarizat_ — Local: a Mac for the macOS leg (mandatory), a Windows box/VM for the Windows leg, a Linux box/VM for AppImage. CI: GitHub Actions free tier gives macos-latest + windows-latest + ubuntu-latest runners — sufficient for single-user release cadence at $0.
- **Linux signing (optional)** — _$0. AppImage has no mandatory signing authority; optional GP_ — Generate a GPG key (`gpg --full-generate-key`) and publish the public key alongside releases; or skip — AppImage is trusted by execute-bit + checksum, not a CA chain.

**步骤**
1. **ENROLL Apple Developer Program TODAY (longest lead time, gates the entire macOS leg). Pay $99, get legal name exactly right, answer any Apple call/email within 48h.**
   ```
   open https://developer.apple.com/programs/enroll
   ```
   — Do this before writing any signing scripts. Approval can take 24-48h best case, 2-7 weeks worst case (early-2026 reports). Everything macOS is blocked until the Developer ID cert exists. Notarization itself is free once enrolled.
2. **START the Windows cert validation in parallel (also slow: OSS-project / identity vetting). Pick Certum SimplySign (cheapest, needs public OSS repo URL — Anselm qualifies) or SSL.com eSigner (no OSS requirement, CI-friendly).**
   ```
   open https://certum.store/open-source-code-signing-on-simplysign.html   # or  open https://www.ssl.com/products/software-integrity/code-signing/ov/
   ```
   — Identity validation takes days. Do NOT plan around Azure Trusted Signing — a non-US/CA individual is ineligible (confirmed in MS Artifact Signing FAQ, 2026). Cloud signing (SimplySign/eSigner) avoids shipping a physical FIPS token internationally.
3. **While both validations are pending, get to first UNSIGNED tri-platform build so the only missing piece is the signature. Build the Go sidecar per target then the Flutter bundle (per ADR 0001 cross-compile is plain go build; per ADR 0004 toolchain is mise).**
   ```
   cd backend && GOOS=darwin GOARCH=arm64 go build -o ../frontend/macos/Runner/sidecar/anselm-server ./cmd/server && cd ../frontend && flutter build macos --release
   ```
   — Repeat with GOOS=windows/linux + flutter build windows/linux. Validates the bundling+spawn path with zero account dependency. Confirms the nested sidecar is in place before you ever touch codesign.
4. **macOS first signed build: once the Developer ID cert is in your login keychain, sign BOTTOM-UP (nested Go sidecar first with Hardened Runtime + the directInstaller entitlements, then the .app), then notarize + staple.**
   ```
   codesign -f -o runtime --timestamp --entitlements sidecar.entitlements -s 'Developer ID Application: <Name> (<TEAMID>)' frontend/build/macos/Build/Products/Release/Anselm.app/Contents/Resources/sidecar/anselm-server
   codesign -f -o runtime --timestamp --entitlements Runner-Release.entitlements -s 'Developer ID Application: <Name> (<TEAMID>)' frontend/build/macos/Build/Products/Release/Anselm.app
   xcrun notarytool submit Anselm.dmg --apple-id <id> --team-id <TEAMID> --password <app-specific-pw> --wait
   xcrun stapler staple Anselm.dmg
   ```
   — Sign the nested sidecar BEFORE the outer .app (no --deep). Notarization round-trip is usually a few minutes once submitted. Budget the entitlement set in step's notes section — wrong entitlements = the downloaded runtimes won't exec at runtime even though the build 'succeeds'.
5. **Windows first signed build: once the OV cert is provisioned in the cloud HSM, sign the Flutter .exe, the bundled server.exe sidecar, and the Inno Setup installer via signtool against the cloud key (eSigner/SimplySign dlib).**
   ```
   signtool sign /tr http://timestamp.acs.microsoft.com /td sha256 /fd sha256 /dlib <esigner-or-simplysign.dll> <Anselm.exe> <sidecar\anselm-server.exe>
   # then build + sign the Inno Setup installer the same way
   ```
   — Inno Setup (full-trust) is the vehicle, NOT MSIX (MSIX AppContainer breaks spawning downloaded runtimes — see packaging section). Expect SmartScreen warnings on early downloads with an OV cert until reputation accrues; this is normal and not fixable by spending more short of an EV cert.
6. **Linux first 'signed' build (cheapest leg): build AppImage via fastforge; optionally GPG-sign. No CA, no cost.**
   ```
   flutter pub global activate fastforge && fastforge release --name dev   # produces AppImage with bundled sidecar; optional: gpg --detach-sign Anselm.AppImage
   ```
   — Linux has NO mandatory signing authority — this leg is unblocked from day one and is the fastest path to a shippable artifact. Use it to prove the end-to-end release pipeline while Apple/Windows validations are still pending.
7. **Wire the auto-update feed on the free, ban-resistant host. Publish signed artifacts + appcast/update-index to GitHub Releases; point the client's feed URL constant at the raw release asset URL.**
   ```
   gh release create v0.1.0 Anselm.dmg Anselm-Setup.exe Anselm.AppImage appcast.xml --title 'Anselm 0.1.0' --notes-file CHANGELOG.md
   ```
   — GitHub Releases is the recommended host precisely because it is portable and not tied to the banned anselm.host domain. The Sparkle/desktop_updater EdDSA signing key is a SEPARATE free key (generated locally, public key in Info.plist / update config) — it's update-payload integrity, orthogonal to the OS code-signing certs.

**与我们(sidecar/下载运行时)**:Two architecture facts reshape the budget and timeline. (1) The BUNDLED Go sidecar (`cmd/server`, pure-Go sqlite, per-GOOS/GOARCH) is a nested Mach-O/PE inside each package and must be signed/notarized in LOCKSTEP with the app at NO extra cost — there is no second cert, but it adds a mandatory extra signing step per platform (sign nested binary BEFORE the outer container, no --deep on macOS; signtool the server.exe alongside Anselm.exe on Windows). Skipping it = Gatekeeper/SmartScreen rejects the whole bundle even though the outer app is signed. Because the Flutter client is a typed projection of the backend contract (ADR 0004), the sidecar can NEVER be updated independently — this rules out

**工具**:`Apple Developer Program + notarytool/stapler`（macOS only） · `Certum Open Source Code Signing (SimplySign cloud)`（Windows only） · `SSL.com IV/OV + eSigner cloud signing`（Windows only） · `Azure Artifact Signing (formerly Trusted Signing)`（Windows only） · `GitHub Releases (+ gh CLI)`（All three (host)） · `fastforge (ex flutter_distributor)`（All three (packaging)） · `desktop_updater OR auto_updater (Sparkle/WinSparkle)`（desktop_updater: all thr）
**坑**:CRITICAL eligibility trap: the obvious cheapest Windows option ($9.99/mo Azure Trusted/Artifact Signing) is UNAVAILABLE to a non-US/Canada individual (MS FAQ, 2026: individuals US/CA only, orgs US/CA/EU/UK only). Plan on Certum SimplySign (~$58/yr, needs public OSS repo) or SSL.com eSigner (~$240/yr, no OSS need) instead — discovering this late blows the timeline. 2026-02-15 rule: ALL public code-signing certs are now max 1-year lifespan AND private keys MUST be on a FIPS 140-2 L2+ token or clou
**成本/时间**:Year-1 minimum (Certum OSS Windows path): $99 (Apple) + ~$58-69 (Certum yr1) + $0 (GitHub host) + $0 (Linux) = ~$157-168 ｜ First-time setup, end-to-end: ~1-3 weeks WALL-CLOCK, dominated by account validation latency, not engineering hours. App

> ⚠ **对抗复审纠正** — **命令**:Mostly correct, with one real bug and several modernization fixes.

1) BUG — `notarytool submit ... --password <app-specific-pw>` (step 4): this still works but is the LEGACY auth path. The section's own prerequisite says to create an App Store Connect API key for "headless notarytool auth in CI," then the command uses ｜ **成本**:Mostly sound, with two corrections.

ACCURATE: Apple $99/yr; notarization free/unlimited; GitHub host $0; Linux AppImage $0; EdDSA update key $0. Azure $9.99/mo basic tier figure matches the live pricing page.

CORRECTION 1 (Certum cloud): the ~$58-69 first-year cloud figure is optimistic. The €69/€29 numbers circulati ｜ **我们特殊点**:Largely holds, but there is one real flaw and one mislabeled mechanism.

WHAT HOLDS:
- Sign the nested Go sidecar BEFORE the outer .app, no --deep, in lockstep, $0 extra cert: correct. Skipping nested signing => Gatekeeper rejects the bundle: correct.
- Whole-bundle update model (no Shorebird/Dart-only OTA because the  ｜ **工具纠正**:`Apple Developer Program + nota`→None on substance. Enrollment-delay warning is well-founded: Apple Developer Forums in ear；`Certum Open Source Code Signin`→Price needs care. The widely-cited ~€69 first-year / €29 renewal figures are for the CARD+；`SSL.com IV/OV + eSigner cloud `→Pricing nuance: the $20/mo is the price of an ADDITIONAL IV/OV signing credential, on top 

---

## windows-sign-installer — Windows release pipeline end-to-end: Authenticode signing (Azure Artifact Signing vs EV cert), signtool + RFC3161 timestamp, signing the bundled Go sidecar, SmartScreen reputation, and the signed Inno Setup installer (vs MSIX) for Anselm's Flutter desktop app

> **目标**:Take `flutter build windows` output (the Release `Runner.exe` + Flutter DLLs + bundled `server.exe` Go sidecar) and turn it into a single, Authenticode-signed, RFC3161-timestamped, SmartScreen-trusted, per-machine-installable `.exe` installer that bundles + co-signs the Go sidecar and updates it in lockstep — distributable to external users without "Unknown Publisher" / blue-warning friction.

**前置(含成本)**
- **Azure subscription + Azure Artifact Signing account (formerly 'Trusted Signing'), Public Trust certificate profile — RECOMMENDED signing identity** — _$9.99/month (Basic SKU: up to 5,000 signatures/mo, 1 cert pr_ — Azure portal > create 'Artifact Signing' resource (region weu/eastus etc.) > complete identity validation (Individual/Business). ELIGIBILITY (verify at apply time): GA but currently limited to US / Canada / EU / UK businesses OR self-employed individuals; the 
- **FALLBACK identity if Azure region-blocked: OV or EV Authenticode code-signing certificate from a CA (SSL.com / Sectigo / Certum / DigiCert)** — _OV ~$200-300/yr; EV ~$249-549/yr (SSL.com EV ~$249, ssl2buy _ — Buy from a CA, pass org/identity vetting (EV = stricter, ~days). EV arrives on a hardware token (YubiKey-class or eToken) or via the CA's cloud-HSM signing service. NOTE since Mar 2024 Microsoft REMOVED EV's instant-SmartScreen-bypass privilege — EV and OV now
- **Windows SDK signtool.exe (>= 10.0.2261.755) + Azure.CodeSigning.Dlib.dll (Microsoft.Trusted.Signing.Client) + .NET 8 runtime** — _Free._ — signtool ships with the Windows 10/11 SDK (install via Visual Studio Installer or standalone SDK). For Azure Artifact Signing, install the dotnet tool / NuGet 'Microsoft.Trusted.Signing.Client' which provides the x64 Azure.CodeSigning.Dlib.dll dispatcher that 
- **Inno Setup 6.x (installer compiler) + ISCC.exe** — _Free (open-source, jrsoftware.org)._ — Install Inno Setup 6 (winget install JRSoftware.InnoSetup, or download). Use ISCC.exe (command-line compiler) in CI. Write one `.iss` script. Pairs cleanly with Flutter Windows build output.
- **Apple-equivalent NOT needed here; but a stable update host (decided separately) — GitHub Releases recommended as ban-resistant appcast + binary host** — _Free (public repo)._ — Use GitHub Releases to host the signed installer + the WinSparkle/auto_updater appcast.xml; avoids dependence on the banned custom domain.

**步骤**
1. **Build the Flutter Windows release bundle AND stage the Go sidecar into it BEFORE signing. flutter build emits build\windows\x64\runner\Release\ containing Runner.exe (rename to anselm.exe via pubspec/CMake) + flutter_windows.dll + plugin DLLs + data\. Cross-compile the pure-Go no-CGO sidecar for windows/amd64 and drop it next to the exe so it ships INSIDE the package.**
   ```
   flutter build windows --release
   set CGO_ENABLED=0
   go build -trimpath -ldflags "-s -w" -o build\windows\x64\runner\Release\server.exe .\backend\cmd\server
   ```
   — server.exe is the localhost sidecar the app spawns (ADR 0004). It MUST live in the same dir set so the Flutter app resolves it via Platform.resolvedExecutable. Pure-Go sqlite (no CGO) means a single self-contained exe, no extra DLLs.
2. **Author metadata.json for Azure Artifact Signing (endpoint = your account's region host, account name, Public-Trust profile name). signtool's /dlib + /dmdf point at this.**
   ```
   {
     "Endpoint": "https://eus.codesigning.azure.net/",
     "CodeSigningAccountName": "anselm-signing",
     "CertificateProfileName": "anselm-public-trust"
   }
   ```
   — Endpoint region must match where you created the account (e.g. eus, weu). Authenticate via env vars AZURE_TENANT_ID / AZURE_CLIENT_ID / AZURE_CLIENT_SECRET (the CI service principal).
3. **SIGN BOTTOM-UP, INSIDE-OUT. Sign the bundled Go sidecar server.exe FIRST, then the app anselm.exe, then (later) the installer. Use SHA256 file digest + an RFC3161 timestamp so signatures stay valid after the cert rotates/expires. With Azure Artifact Signing the cert rotates daily server-side but timestamped binaries remain valid forever.**
   ```
   signtool sign /v /fd SHA256 /td SHA256 ^
     /tr http://timestamp.acs.microsoft.com ^
     /dlib "%USERPROFILE%\.dotnet\tools\...\Azure.CodeSigning.Dlib.dll" ^
     /dmdf metadata.json ^
     build\windows\x64\runner\Release\server.exe
   
   signtool sign /v /fd SHA256 /td SHA256 ^
     /tr http://timestamp.acs.microsoft.com /dlib ... /dmdf metadata.json ^
     build\windows\x64\runner\Release\anselm.exe
   ```
   — /tr = RFC3161 timestamp URL (Azure: http://timestamp.acs.microsoft.com); /td SHA256 = timestamp digest alg. EVERY shipped PE must be signed: sidecar exe, app exe, every first-party DLL you author. Third-party Flutter/plugin DLLs are already signed or benign; signing the top-level exes + sidecar + installer is what SmartScreen + UAC evaluate. With an EV/OV CA cert instead, swap /dlib+/dmdf for /f c
4. **Write the Inno Setup .iss script: per-MACHINE install (Program Files, requires admin once), bundles the WHOLE Release dir incl. server.exe, and configures signtool as a named sign tool so Inno signs the generated setup.exe AND the uninstaller automatically.**
   ```
   ; anselm.iss
   [Setup]
   AppId={{<stable-guid>}}
   AppName=Anselm
   AppVersion=1.4.0
   DefaultDirName={autopf}\Anselm        ; {autopf}=Program Files (per-machine)
   PrivilegesRequired=admin               ; per-machine; use 'lowest'+{autopf}->{userpf} for per-user
   ArchitecturesAllowed=x64compatible
   ArchitecturesInstallIn64BitMode=x64compatible
   WizardStyle=modern
   OutputBaseFilename=anselm-setup-1.4.0-x64
   SignTool=azuresign
   SignedUninstaller=yes
   
   [Files]
   Source: "build\windows\x64\runner\Release\*"; DestDir: "{app}"; Flags: recursesubdirs ignoreversion
   
   [Icons]
   Name: "{autoprograms}\Anselm"; Filename: "{app}\anselm.exe"
   Name: "{autodesktop}\Anselm"; Filename: "{app}\anselm.exe"; Tasks: desktopicon
   
   [Run]
   Filename: "{app}\anselm.exe"; Description: "Launch Anselm"; Flags: nowait postinstall skipifsilent
   ```
   — SignTool=azuresign references a named sign-tool command registered with ISCC (next step). SignedUninstaller=yes makes Inno sign the embedded uninstaller too. Per-machine (admin + {autopf}) is RECOMMENDED for us — see ourSpecifics. For per-user (no admin prompt) set PrivilegesRequired=lowest and DefaultDirName={autopf} resolves to per-user Program Files; tradeoff: installs only for current user.
5. **Register the named sign tool command for ISCC and compile. ISCC's /S<name>=<command> maps the SignTool=azuresign reference to a real signtool invocation; $f is the file Inno wants signed (the setup.exe / uninstaller), $p is params. Inno then signs the produced installer + uninstaller with the SAME Azure identity.**
   ```
   ISCC.exe ^
     /Sazuresign="signtool sign /fd SHA256 /td SHA256 /tr http://timestamp.acs.microsoft.com /dlib \"path\Azure.CodeSigning.Dlib.dll\" /dmdf metadata.json $f" ^
     anselm.iss
   ```
   — $f expands to the file Inno passes; $p to extra params; $q quotes. Inno calls this once for setup.exe and once for the uninstaller (because SignedUninstaller=yes). Result: anselm-setup-1.4.0-x64.exe, signed + timestamped. Inno allows ONE sign tool per script — fine, we sign the inner PEs in step 3 ourselves and let Inno sign only the installer/uninstaller.
6. **Verify every signature + timestamp before publishing. signtool verify confirms a valid Authenticode chain + timestamp on each PE (sidecar, app, installer).**
   ```
   signtool verify /pa /v build\windows\x64\runner\Release\server.exe
   signtool verify /pa /v build\windows\x64\runner\Release\anselm.exe
   signtool verify /pa /v Output\anselm-setup-1.4.0-x64.exe
   ```
   — /pa = use Default Authenticode verification policy (what the OS uses for app trust, not driver policy). A PASS here means no 'Unknown Publisher' UAC dialog; the publisher name from your cert shows in blue.
7. **Publish to GitHub Releases (ban-resistant host) and let SmartScreen reputation accrue. New signing identity = SmartScreen may still warn until download volume builds; the signature ensures the warning shows YOUR verified publisher name (not 'Unknown Publisher') and reputation now accrues to your IDENTITY (Azure Artifact Signing) across all future binaries, surviving daily cert rotation.**
   ```
   gh release create v1.4.0 Output\anselm-setup-1.4.0-x64.exe --notes-file CHANGELOG.md
   ```
   — SmartScreen behavior 2025/2026: since Mar 2024 EV no longer gives instant bypass; OV/EV/Azure-identity all build reputation by volume. Azure Artifact Signing's advantage is reputation is tied to your stable IDENTITY not a per-cert serial, so it accumulates faster across releases. Early external users may see one 'More info > Run anyway' click until volume crosses Microsoft's threshold.
8. **Wire it into CI (GitHub Actions, windows-latest). Use the official azure/trusted-signing-action OR install the dlib + run signtool manually, with the service-principal secrets. Lockstep: the same workflow builds the sidecar, signs it, signs the app, compiles+signs the installer — one atomic versioned artifact.**
   ```
   # .github/workflows/release-windows.yml (key step)
   - uses: azure/trusted-signing-action@v0
     with:
       azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
       azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
       azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}
       endpoint: https://eus.codesigning.azure.net/
       trusted-signing-account-name: anselm-signing
       certificate-profile-name: anselm-public-trust
       files-folder: build\windows\x64\runner\Release
       files-folder-filter: exe,dll
   ```
   — Run flutter build + go build BEFORE this step; run the Inno compile (step 5) AFTER (signing inner PEs first, then build the installer, then sign the installer). Store the 3 Azure secrets in repo/org secrets. This guarantees app + sidecar always ship + version + sign together (no drift).

**与我们(sidecar/下载运行时)**:Two Anselm-specific facts drive the Windows pipeline: (1) the BUNDLED Go sidecar server.exe, and (2) FIRST-USE download+execute of language runtimes (python/node/uv/dotnet) via directInstaller (ADR 0001).\n\nBUNDLED SIDECAR: server.exe is a real PE spawned as a child on localhost. On Windows there is NO 'deep'/nested-bundle signing concept like macOS — you simply sign EACH PE independently with the SAME identity. So sign server.exe explicitly (step 3) BEFORE the app and BEFORE the installer; do NOT rely on the installer signature to cover it. If server.exe is unsigned, it won't trigger a UAC 'Unknown Publisher' prompt on spawn (no elevation), but it WILL hurt SmartScreen/AV heuristics and lo

**工具**:`Azure Artifact Signing (formerly 'Trusted Signing')`（Windows code signing (Au） · `signtool.exe (Windows SDK) + Microsoft.Trusted.Signing.Client (Azure.CodeSigning.Dlib.dll)`（Windows.） · `Inno Setup 6`（Produces a Windows .exe ） · `MSIX (Windows packaging) — REJECTED for Anselm`（Windows.） · `azure/trusted-signing-action (GitHub Action)`（CI (windows-latest).）
**坑**:1) Azure Artifact Signing is REGION-LOCKED (US/CA/EU/UK as of 2025/2026). Verify eligibility for the user's jurisdiction BEFORE committing; if blocked, the EV/OV CA path is the fallback (token-based 2FA for EV).\n2) EV is NO LONGER a SmartScreen shortcut — since Mar 2024 EV lost instant-bypass; do NOT pay the EV premium expecting zero warnings. Reputation builds by download volume for OV/EV/Azure alike. Azure-identity reputation accrues fastest because it's identity-scoped.\n3) ALWAYS RFC3161-ti
**成本/时间**:~$10/month recurring (Azure Artifact Signing Basic, $9.99/mo, 5,000 sigs) + $0 for signtool/Inno Setup/GitHub Releases.  ｜ First-time setup: 0.5-3 days, dominated by Azure Artifact Signing IDENTITY VALIDATION (1 hour to several days depending 

> ⚠ **对抗复审纠正** — **命令**:Mostly correct, with three concrete errors and one stale reference:

1) GITHUB ACTION IS STALE (step 8) — HARD BREAK. The playbook uses `azure/trusted-signing-action@v0` with input `trusted-signing-account-name`. As of Jan 2026 the service + repo were renamed: the repo is now `Azure/artifact-signing-action` (current `@ ｜ **成本**:Mostly accurate. CORRECTIONS:

1) 458-day cert validity is WRONG. CA/Browser Forum Ballot CSC-31 (adopted Nov 17 2025) sets the max at 460 days, effective ~Mar 1 2026; some CAs (DigiCert/SSL.com) implement it as 459 days effective Feb 23 2026 for operational margin. Use '~459-460 days' — not 458. Direction and date are ｜ **我们特殊点**:The Anselm-specific handling is fundamentally SOUND — notably more correct than typical playbooks. Verified points:

1) NO nested-bundle/deep-signing concept on Windows (unlike macOS): CORRECT. You sign each PE independently with the same identity. Signing server.exe bottom-up before the app before the installer is the ｜ **工具纠正**:`Azure Artifact Signing (former`→Eligibility is stated too loosely. The playbook says US/CA/EU/UK 'businesses OR self-emplo；`signtool.exe + Microsoft.Trust`→None. Ensure signtool and the dlib are the SAME architecture (x64); the playbook implies x；`Inno Setup 6`→Fix the per-user path wording ({autopf} non-admin → %LOCALAPPDATA%\Programs, not 'per-user

---

## linux-package-desktop

> **目标**:Take `flutter build linux` output to an INSTALLABLE, self-updating, integrity-verified Anselm artifact on Linux: a primary AppImage (single-file, no sandbox — matches our spawn-sidecar + download-runtime model) that embeds the per-arch Go sidecar, registers a .desktop entry + icon + AppStream MetaInfo so it appears in GNOME Software / KDE Discover, carries GPG signature + SHA256SUMS for integrity, and self-updates via zsync delta with gh-releases-zsync update-information (GitHub Releases as host = ban-resistant, no custom domain). Secondary .deb/.rpm offered only when distro-native install is requested. Flatpak/Snap explicitly deprioritized.

**前置(含成本)**
- **Linux x86_64 build host (native or Ubuntu 22.04 in WSL2 / GitHub Actions ubuntu-22.04 runner) with Flutter Linux desktop deps (clang, cmake, ninja-build, pkg-config, libgtk-3-dev, liblzma-dev)** — _$0 (GitHub Actions ubuntu-latest is free for public repos; ~_ — sudo apt-get install -y clang cmake ninja-build pkg-config libgtk-3-dev liblzma-dev ; mise provides the flutter+go toolchain per ADR 0005. For arm64 Linux artifacts you need an arm64 runner/box — there is no x64->arm64 Flutter cross-build.
- **appimagetool (AppImage/appimagetool, x86_64 + aarch64 AppImages)** — _$0 (open source, MIT/custom). Actively maintained 2025._ — wget https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage ; chmod +x. Bundles mksquashfs. Pin a specific release tag for reproducibility rather than 'continuous'.
- **zsyncmake (from the zsync package)** — _$0 (Artistic License)._ — sudo apt-get install -y zsync . appimagetool auto-detects it and emits the .AppImage.zsync delta file when -u/update-information is set. Without it, no delta update file is produced.
- **AppStream + desktop-file-utils validators** — _$0._ — sudo apt-get install -y appstream desktop-file-utils  (gives appstreamcli + desktop-file-validate).
- **GPG key for release signing** — _$0 (self-managed key; NO CA needed on Linux — unlike macOS D_ — gpg --full-generate-key (ed25519 or RSA4096); publish the public key on the GitHub repo + a keyserver. Store the secret key as a GitHub Actions secret for CI signing.
- **GitHub repository with Releases enabled (the ban-resistant update host)** — _$0. Free Releases + asset hosting. This is the appcast/updat_ — Existing repo. Releases assets are served from objects.githubusercontent.com / a CDN; zsync ranged-GET works against them. Optionally mirror to a generic CDN/S3 with a plain zsync| URL as a fallback transport.

**步骤**
1. **Build the Flutter Linux release bundle on the target arch. Output lands in build/linux/<arch>/release/bundle/ containing the `anselm` ELF, lib/, and data/ (flutter_assets, icudtl).**
   ```
   flutter build linux --release
   # artifact dir: build/linux/x64/release/bundle/   (or arm64)
   ```
   — No cross-compile: x64 box builds x64, arm64 box builds arm64. The bundle is relocatable (rpath $ORIGIN/lib), so it drops straight into an AppDir.
2. **Cross-build the Go sidecar for this GOOS/GOARCH (pure-Go sqlite, no CGO per ADR 0001) and stage it INTO the bundle so it ships inside the AppImage and updates in lockstep.**
   ```
   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o build/linux/x64/release/bundle/anselm-server ./backend/cmd/server
   chmod 0755 build/linux/x64/release/bundle/anselm-server
   ```
   — VERSION must match the Flutter pubspec build number — single source of truth so the lockstep-update invariant holds. Dart locates it at runtime via Platform.resolvedExecutable's dir + '/anselm-server'.
3. **Assemble the AppDir tree. Copy the bundle under usr/bin, then add desktop integration files at the freedesktop-standard paths. The AppImage runtime mounts this read-only and runs AppRun.**
   ```
   APPDIR=AppDir; ID=host.anselm.Anselm
   mkdir -p $APPDIR/usr/bin $APPDIR/usr/share/applications \
     $APPDIR/usr/share/metainfo $APPDIR/usr/share/icons/hicolor/{256x256,512x512,scalable}/apps
   cp -r build/linux/x64/release/bundle/* $APPDIR/usr/bin/
   chmod 0755 $APPDIR/usr/bin/anselm $APPDIR/usr/bin/anselm-server
   ```
   — Use a reverse-DNS id (host.anselm.Anselm) for every filename: $ID.desktop, $ID.metainfo.xml, icon $ID.png. AppStream REQUIRES the .desktop basename == <id> + '.desktop'. Keep the existing anselm.host reverse-DNS even though the site was banned — the id is just a namespace string, not a fetched URL.
4. **Write the .desktop entry (also placed at AppDir root for the AppImage spec) and validate it. StartupWMClass must match the GTK app_id Flutter sets ('anselm') so the taskbar groups the window with its launcher icon.**
   ```
   cat > $APPDIR/usr/share/applications/$ID.desktop <<'EOF'
   [Desktop Entry]
   Type=Application
   Version=1.0
   Name=Anselm
   GenericName=Agentic Workflow Platform
   Comment=Local-first agentic workflow platform
   Exec=AppRun %U
   Icon=host.anselm.Anselm
   Terminal=false
   Categories=Development;Utility;
   StartupWMClass=anselm
   StartupNotify=true
   X-AppImage-Version=1.0.0
   EOF
   desktop-file-validate $APPDIR/usr/share/applications/$ID.desktop
   cp $APPDIR/usr/share/applications/$ID.desktop $APPDIR/$ID.desktop
   ```
   — Exec=AppRun (not the raw binary) so single-instance/deep-link args flow through our launcher. desktop-file-validate must exit 0 — GNOME/KDE silently drop malformed entries. Categories drive the menu placement.
5. **Write the AppStream MetaInfo XML (makes it list in GNOME Software / KDE Discover) and validate. Includes <releases> (drives the in-software-center update banner), screenshots, content_rating, branding. <id> matches the .desktop id exactly.**
   ```
   cat > $APPDIR/usr/share/metainfo/$ID.metainfo.xml <<'EOF'
   <?xml version="1.0" encoding="UTF-8"?>
   <component type="desktop-application">
     <id>host.anselm.Anselm</id>
     <metadata_license>CC0-1.0</metadata_license>
     <project_license>LicenseRef-proprietary</project_license>
     <name>Anselm</name>
     <summary>Local-first agentic workflow platform</summary>
     <developer id="host.anselm"><name>Anselm</name></developer>
     <description>
       <p>Anselm runs agentic workflows entirely on your machine.</p>
       <ul><li>Functions, Handlers, Agents, Workflows</li><li>Durable execution</li></ul>
     </description>
     <launchable type="desktop-id">host.anselm.Anselm.desktop</launchable>
     <url type="homepage">https://github.com/<owner>/anselm</url>
     <url type="bugtracker">https://github.com/<owner>/anselm/issues</url>
     <screenshots>
       <screenshot type="default"><image>https://github.com/<owner>/anselm/raw/main/docs/assets/shot1.png</image><caption>Main interface</caption></screenshot>
     </screenshots>
     <releases>
       <release version="1.0.0" date="2026-06-25"><description><p>Initial release.</p></description></release>
     </releases>
     <content_rating type="oars-1.1" />
   </component>
   EOF
   appstreamcli validate --pedantic $APPDIR/usr/share/metainfo/$ID.metainfo.xml
   ```
   — metadata_license MUST be present (use CC0-1.0 for the metainfo file itself) or distros drop the metadata. Screenshot <image> must be a real reachable URL — host it in the repo (raw.githubusercontent), NOT the banned domain. <releases> dates cannot be future-dated. project_license: use a real SPDX id or LicenseRef-* for proprietary.
6. **Install icons at hicolor sizes (PNG at 256/512, optional SVG in scalable). Software centers need a 256px+ icon or they flag a quality warning.**
   ```
   cp icons/anselm-256.png $APPDIR/usr/share/icons/hicolor/256x256/apps/$ID.png
   cp icons/anselm-512.png $APPDIR/usr/share/icons/hicolor/512x512/apps/$ID.png
   cp icons/anselm.svg    $APPDIR/usr/share/icons/hicolor/scalable/apps/$ID.svg
   cp icons/anselm-256.png $APPDIR/$ID.png   # top-level icon the AppImage runtime uses
   ```
   — Icon basename == id. The AppImage spec also wants a copy (or symlink) of the icon + .desktop at the AppDir ROOT; appimagetool reads them to build the thumbnail/desktop integration.
7. **Write AppRun — the entrypoint the AppImage mounts and execs. It puts our bundled binaries on PATH and launches the Flutter app, which in turn spawns anselm-server as the localhost sidecar.**
   ```
   cat > $APPDIR/AppRun <<'EOF'
   #!/bin/sh
   HERE=$(dirname "$(readlink -f "$0")")
   export PATH="$HERE/usr/bin:$PATH"
   export LD_LIBRARY_PATH="$HERE/usr/bin/lib:$LD_LIBRARY_PATH"
   exec "$HERE/usr/bin/anselm" "$@"
   EOF
   chmod 0755 $APPDIR/AppRun
   ```
   — The Flutter Linux loader already sets rpath=$ORIGIN/lib, so LD_LIBRARY_PATH is belt-and-suspenders. AppRun is also the Exec target in the .desktop so deep-link/file args ($@) propagate.
8. **Build the AppImage with embedded gh-releases-zsync update-information. appimagetool auto-runs zsyncmake and emits both the .AppImage and the .AppImage.zsync delta file. This single string wires the entire Linux auto-update path to GitHub Releases — no custom domain.**
   ```
   export VERSION=1.0.0 ARCH=x86_64
   export UPDATE_INFORMATION="gh-releases-zsync|<owner>|anselm|latest|Anselm-*-x86_64.AppImage.zsync"
   ./appimagetool-x86_64.AppImage \
     --updateinformation "$UPDATE_INFORMATION" \
     --sign --sign-key <GPG_KEY_ID> \
     AppDir Anselm-$VERSION-$ARCH.AppImage
   # produces: Anselm-1.0.0-x86_64.AppImage  +  Anselm-1.0.0-x86_64.AppImage.zsync
   ```
   — gh-releases-zsync|owner|repo|latest|<glob>: AppImageUpdate hits the GitHub Releases API for the 'latest' release, matches the glob, and zsync-deltas only the changed squashfs blocks (the sidecar swap + Flutter assets), saving bandwidth. --sign embeds a detached GPG signature; record VERSION in the filename so update detection is unambiguous. Set ARCH env so appimagetool stamps the right arch.
9. **Generate checksums + a detached signature alongside the artifact for trust-on-first-use verification (Linux has no Gatekeeper/SmartScreen; the published GPG public key + SHA256SUMS IS the trust chain).**
   ```
   sha256sum Anselm-1.0.0-x86_64.AppImage Anselm-1.0.0-x86_64.AppImage.zsync > SHA256SUMS
   gpg --batch --yes --detach-sign --armor -o SHA256SUMS.asc SHA256SUMS
   # verify embedded AppImage signature:
   ./Anselm-1.0.0-x86_64.AppImage --appimage-signature
   ```
   — Two signature layers: (a) GPG embedded IN the AppImage via --sign (validated by AppImageKit's validate tool / AppImageUpdate), (b) detached SHA256SUMS.asc for users who verify before first run. Publish your public key in the repo README + a keyserver.
10. **Publish to GitHub Releases under a 'latest'-resolvable tag. The .AppImage, .AppImage.zsync, SHA256SUMS, SHA256SUMS.asc all become release assets. This is the ban-resistant update host the embedded gh-releases-zsync string targets.**
   ```
   gh release create v1.0.0 \
     Anselm-1.0.0-x86_64.AppImage \
     Anselm-1.0.0-x86_64.AppImage.zsync \
     SHA256SUMS SHA256SUMS.asc \
     --title 'Anselm 1.0.0' --notes-file CHANGELOG-1.0.0.md
   ```
   — For each new release, re-upload a .zsync whose filename glob still matches the embedded pattern (Anselm-*-x86_64.AppImage.zsync). AppImageUpdate resolves 'latest' via the GitHub API, so the user's installed AppImage finds the new one with zero config. If GitHub is ever blocked too, the SAME AppDir can be re-stamped with a plain zsync|https://<cdn>/Anselm-x86_64.AppImage.zsync transport — portable 
11. **Ship the in-app updater that drives the zsync delta (Linux has no Sparkle/WinSparkle — auto_updater has no Linux backend). Bundle the AppImageUpdate CLI or call libappimageupdate; on 'update available' it delta-downloads, then atomically replaces the running AppImage file.**
   ```
   # AppImageUpdate CLI (bundle appimageupdatetool, ~6MB):
   appimageupdatetool --check-for-update Anselm-1.0.0-x86_64.AppImage  # exit 1 = update available
   appimageupdatetool Anselm-1.0.0-x86_64.AppImage                     # performs the zsync delta + replace
   ```
   — Because the Go sidecar lives INSIDE the squashfs, the zsync delta updates app + sidecar atomically in one file swap — the lockstep invariant is automatic on Linux (unlike a side-by-side sidecar). After replace, the app must restart to respawn the new sidecar. desktop_updater (pub.dev, 3-platform) is the Flutter-native alternative but does a full verified-zip replace, not a delta — prefer zsync for
12. **(Conditional) Offer .deb/.rpm ONLY when users ask for distro-native install (system menu integration without AppImaged, apt/dnf upgrade path). Drive from the SAME staged bundle. Skip for v1 unless requested.**
   ```
   # fastforge orchestrates deb/rpm from one config (build/linux/x64/release/bundle/):
   fastforge package --platform linux --targets deb,rpm
   # deb installs the bundle to /opt/anselm, the .desktop to /usr/share/applications, metainfo to /usr/share/metainfo
   ```
   — deb/rpm have NO built-in auto-update unless you also run an apt/dnf repo (extra hosting + GPG repo signing) — that's why AppImage+zsync is primary. fastforge (ex flutter_distributor, actively maintained) wraps the deb/rpm tooling so you don't hand-write control/spec files. The bundled sidecar + the same .desktop/metainfo/icon files are reused verbatim.

**与我们(sidecar/下载运行时)**:BUNDLED GO SIDECAR: anselm-server is staged INTO the Flutter bundle (step 2) before AppDir assembly, so it lives inside the AppImage's read-only squashfs. This is the cleanest of the three OSes for the lockstep-update invariant: a zsync delta replaces the single .AppImage file, so app + sidecar always update together atomically — there is no side-by-side binary to drift. Exec bit must be 0755 on anselm-server inside the AppDir (squashfs preserves it); Dart resolves it via Platform.resolvedExecutable's directory at runtime. Version-stamp the sidecar (-ldflags -X main.version) to MATCH the pubspec build number so a health probe can assert app==sidecar after an update. UNLIKE macOS, NO code-sig

**工具**:`appimagetool (AppImage/appimagetool)`（Linux (build host). Prod） · `zsync / zsyncmake`（Linux.） · `AppImageUpdate / appimageupdatetool`（Linux.） · `appstreamcli (ximion/appstream)`（Linux (apt: appstream).） · `desktop-file-validate (desktop-file-utils)`（Linux (apt: desktop-file） · `fastforge (ex flutter_distributor)`（Cross-platform; here use） · `desktop_updater (pub.dev)`（macOS/Windows/Linux.）
**坑**:1) NO Flutter Linux cross-compile: you MUST build x64 on an x64 box and arm64 on an arm64 box (matters for CI matrix). 2) The AppImage squashfs is READ-ONLY — downloaded runtimes and SQLite MUST go to ANSELM_DATA_DIR (XDG), never inside the mount; and that dir must NOT be on a noexec mount or directInstaller's runtimes fail to exec. 3) AppStream <id> and the .desktop basename MUST be identical (host.anselm.Anselm + .desktop) or it won't appear in software centers; metadata_license is mandatory. 
**成本/时间**:$0 in tooling and signing. Linux is the cheapest of the three platforms: no Apple Developer Program ($99/yr), no Windows ｜ First-time setup: ~1.5-2.5 days to wire the full AppDir assembly + sidecar staging + .desktop/metainfo/icon + GPG signin

> ⚠ **对抗复审纠正** — **命令**:Mostly correct, with command-syntax errors:

1. STEP 2 winsparkle-tool is WRONG. Playbook shows `winsparkle-tool.exe generate-key > winsparkle.key`. Real syntax (verified vs vslavik/winsparkle README): `winsparkle-tool generate-key --file private.key` — the PRIVATE key is written to the file via `--file`, and the PUBLI ｜ **成本**:Accurate for 2025-2026. Apple Developer Program $99/yr is current and correctly noted as SHARED with the macOS signing section (not new cost). All update tooling (auto_updater/Sparkle/WinSparkle/AppImageUpdate/zsync/fastforge) genuinely $0 MIT/open-source. GitHub Releases hosting + egress free on public repos — correct ｜ **我们特殊点**:Core architecture is SOUND and reasoning correct, with real flaws to harden:

HOLDS UP:
- Lockstep-via-whole-bundle-swap is genuinely automatic. Sparkle swaps the entire .app (.zip), AppImageUpdate swaps the whole AppImage (zsync); the nested anselm-server at .app/Contents/MacOS/ travels along for free. Rejecting Shore ｜ **工具纠正**:`auto_updater (Flutter)`→Accurate. Low cadence flagged correctly. Nuance: it's a thin wrapper over a PINNED Sparkle；`Sparkle 2.x (generate_keys/gen`→Accurate. CFBundleVersion (build number, not marketing string) comparison correct and a re；`WinSparkle + winsparkle-tool`→Tool real, but generate-key COMMAND in step 2 is wrong (stdout redirect instead of --file)

---

## App identity + packaging metadata (bundle IDs, AUMID/MSIX identity, Linux app-id, icons, display name, anselm:// scheme, version scheme, min-OS) for the tri-platform Anselm Flutter desktop release

> **目标**:Lock down ONE consistent identity surface across macOS/Windows/Linux + the bundled Go sidecar, so that every downstream release step (signing, notarization, AUMID-keyed notifications, anselm:// deep links, data-dir path, auto-update appcast matching) has a stable, correct, ban-resistant set of identifiers and metadata. Concretely: fix the current `host.anselm.anselm` identity (which both double-stutters and reverses to the BANNED anselm.host domain), generate all per-OS icons from one source, declare correct min-OS + version/build scheme, and register anselm:// — all before the first signed build.

**前置(含成本)**
- **A registered reverse-DNS namespace you actually control (NOT anselm.host, which got banned). Pick a domain you own or a code-hosting namespace — e.g. io.github.<user> or app.getanselm — and use it as the bundle-ID/app-id prefix everywhere.** — _$0–$12/yr (domain optional; io.github.<user> is free and ban_ — Either register a domain (Cloudflare/Namecheap ~$10/yr) OR adopt io.github.<yourGithubUser> which Flathub/Flatpak explicitly bless for projects without a domain. The prefix only needs to be a namespace you won't lose — it never has to resolve to a live site.
- **Apple Developer Program membership (already a confirmed decision) — needed so the CFBundleIdentifier is registered to your Team and is signable/notarizable.** — _$99/yr_ — developer.apple.com/programs — enroll, then the bundle ID is registered implicitly on first Developer ID signing (no separate App ID record needed for Developer-ID/non-MAS distribution).
- **flutter_launcher_icons (dev_dependency) for one-source icon generation across macOS/Windows/Linux.** — _$0 (MIT)_ — Add `flutter_launcher_icons: ^0.14.4` (latest, released 2025-06-10) under dev_dependencies in pubspec.yaml.
- **A single high-res master icon: 1024x1024 PNG, square, no alpha-edge bleed (macOS wants squircle masking handled by you; provide full-bleed art). You already have assets/brand/anselm-icon.svg as the design source.** — _$0_ — Render assets/brand/anselm-icon.svg to a 1024x1024 PNG (e.g. `rsvg-convert -w 1024 -h 1024` or Inkscape export) → assets/brand/icon_1024.png.

**步骤**
1. **DECIDE the reverse-DNS prefix and freeze the full identity matrix. Current repo uses `host.anselm.anselm` — REJECT it: it double-stutters AND reverses to the banned anselm.host domain (a notarization/reputation liability if Apple/MS ever cross-reference). Choose a ban-resistant prefix you control, e.g. `app.getanselm` (if you own getanselm.app) or `io.github.<user>` (free, blessed by Flatpak/Flathub). Final matrix: APP id = <prefix>.anselm ; SIDECAR id = <prefix>.sidecar ; AUMID (Windows) = <prefix>.anselm ; Linux app-id = <prefix>.anselm ; macOS data-dir leaf auto-derives to <prefix>.anselm.**
   ```
   # Example chosen identity (replace app.getanselm with your namespace):
   # APP_ID=app.getanselm.anselm
   # SIDECAR_ID=app.getanselm.sidecar
   # AUMID=app.getanselm.anselm
   # LINUX_APP_ID=app.getanselm.anselm
   ```
   — This is the load-bearing decision of the whole section. Frozen BEFORE first external release because macOS data-dir + downloaded-runtime store path derive from it (README data-dir open-Q #1: 'unfixable after release'). Avoid anselm.host-derived strings entirely.
2. **Set the macOS app identity in AppInfo.xcconfig (single source for the Runner target). Replace PRODUCT_BUNDLE_IDENTIFIER and the stutter; set a human PRODUCT_NAME for window title/Finder and a clean copyright that does NOT reference the banned domain.**
   ```
   # frontend/macos/Runner/Configs/AppInfo.xcconfig
   PRODUCT_NAME = Anselm
   PRODUCT_BUNDLE_IDENTIFIER = app.getanselm.anselm
   PRODUCT_COPYRIGHT = Copyright © 2026 The Anselm Project. All rights reserved.
   ```
   — PRODUCT_NAME becomes CFBundleName (the .app folder + menu name; keep ≤15 chars). Info.plist already references $(PRODUCT_NAME)/$(PRODUCT_BUNDLE_IDENTIFIER) so no plist edit needed for these. Also fix the 3 RunnerTests PRODUCT_BUNDLE_IDENTIFIER lines in project.pbxproj from host.anselm.anselm.RunnerTests to <prefix>.anselm.RunnerTests.
3. **Add CFBundleDisplayName + register the anselm:// URL scheme in the macOS Info.plist (CFBundleURLTypes). Display name shows the full product name where the OS has room (Spotlight, About); the URL type lets the OS route anselm://flowrun/<id> to the running single instance (app_links receives it per README deep-links cluster line ~278).**
   ```
   <!-- add inside macos/Runner/Info.plist <dict> -->
   <key>CFBundleDisplayName</key>
   <string>Anselm</string>
   <key>CFBundleURLTypes</key>
   <array>
     <dict>
       <key>CFBundleURLName</key>
       <string>app.getanselm.anselm</string>
       <key>CFBundleURLSchemes</key>
       <array><string>anselm</string></array>
     </dict>
   </array>
   ```
   — CFBundleURLName conventionally = the bundle id. The scheme `anselm` is private/custom (no Associated Domains needed — README line ~283 explicitly defers HTTPS universal links since Anselm is local-first, no web property).
4. **Confirm/raise the macOS minimum OS to 10.15 (Catalina). Flutter's current template + 2025 migration (flutter/flutter#167745, PR #168101) sets MACOSX_DEPLOYMENT_TARGET=10.15; the repo's pbxproj already has 10.15 (lines 560/642/692) and Info.plist maps LSMinimumSystemVersion=$(MACOSX_DEPLOYMENT_TARGET). Verify no Podfile/xcconfig still says 10.14.**
   ```
   grep -rn 'MACOSX_DEPLOYMENT_TARGET\|platform :osx\|MinimumOSVersion' frontend/macos/ | grep -v 10.15
   ```
   — Empty output = already consistent at 10.15. 10.15 is the right floor for a 2026 release (10.14 usage is negligible per Flutter team). Hardened Runtime (the signing posture) requires a modern OS anyway.
5. **Set Windows identity: AUMID in main.cpp (before UI), version-info strings in Runner.rc, and the BINARY_NAME. Replace the CompanyName/copyright that reference host.anselm. The AUMID is the SAME reverse-DNS string as the bundle id, so toasts + taskbar group correctly even with the bundled sidecar child process.**
   ```
   // frontend/windows/runner/main.cpp — add near top of wWinMain, before window.Create:
   #include <shobjidl.h>
   SetCurrentProcessExplicitAppUserModelID(L"app.getanselm.anselm");
   
   // frontend/windows/runner/Runner.rc — version block:
   VALUE "CompanyName", "The Anselm Project" "\0"
   VALUE "FileDescription", "Anselm" "\0"
   VALUE "ProductName", "Anselm" "\0"
   VALUE "LegalCopyright", "Copyright (C) 2026 The Anselm Project." "\0"
   VALUE "OriginalFilename", "anselm.exe" "\0"
   ```
   — FileVersion/ProductVersion are already wired to VERSION_AS_STRING (derived from pubspec version by Flutter). If you later ship MSIX (NOT the chosen vehicle — README line ~346 picks Inno Setup for full-trust child-process support), the Package Identity Name must ALSO match this reverse-DNS id; staying consistent now keeps that door open. The Inno Setup script must set the shortcut's AppUserModelID 
6. **Set Linux identity: APPLICATION_ID in CMakeLists.txt to the reverse-DNS app-id, fix the window title in my_application.cc, and author the freedesktop .desktop + AppStream metainfo (named after the app-id). app-id MUST be D-Bus-valid reverse-DNS and match macOS/Windows.**
   ```
   # frontend/linux/CMakeLists.txt
   set(APPLICATION_ID "app.getanselm.anselm")
   
   # frontend/linux/runner/my_application.cc — titles:
   gtk_header_bar_set_title(header_bar, "Anselm");
   gtk_window_set_title(window, "Anselm");
   
   # linux/packaging/app.getanselm.anselm.desktop (authored once, shipped in AppImage/deb):
   [Desktop Entry]
   Name=Anselm
   Exec=anselm
   Icon=app.getanselm.anselm
   Type=Application
   Categories=Development;Utility;
   StartupWMClass=anselm
   ```
   — BINARY_NAME stays `anselm` (the on-disk exe). The .desktop Icon= value references an icon installed to hicolor/<size>/apps/app.getanselm.anselm.png (+ scalable SVG). StartupWMClass must match the GTK app id for correct taskbar icon association. Ship a <app-id>.metainfo.xml too if targeting Flathub.
7. **Generate ALL per-OS icons from the one 1024px master via flutter_launcher_icons. This overwrites the existing macOS appiconset PNGs and windows app_icon.ico from a single source, guaranteeing visual identity consistency.**
   ```
   # render master from your SVG once:
   rsvg-convert -w 1024 -h 1024 frontend/assets/brand/anselm-icon.svg > frontend/assets/brand/icon_1024.png
   
   # pubspec.yaml dev_dependencies + config block:
   #   dev_dependencies:
   #     flutter_launcher_icons: ^0.14.4
   #   flutter_launcher_icons:
   #     image_path: "assets/brand/icon_1024.png"
   #     macos: { generate: true, image_path: "assets/brand/icon_1024.png" }
   #     windows: { generate: true, image_path: "assets/brand/icon_1024.png", icon_size: 256 }
   #     # linux not auto-handled — see notes
   
   cd frontend && dart run flutter_launcher_icons
   ```
   — macOS: writes Assets.xcassets/AppIcon.appiconset (16/32/64/128/256/512/1024 @1x/@2x) — Xcode compiles these into the .icns at build. Windows: writes runner/resources/app_icon.ico (multi-res up to 256). LINUX: flutter_launcher_icons' Linux support is thin — manually export PNGs (16,32,48,64,128,256,512) into linux/packaging/icons/hicolor/<size>x<size>/apps/app.getanselm.anselm.png + a scalable SVG,
8. **Define the version/build-number scheme in pubspec.yaml (single source — Flutter propagates to all three platforms). Use SemVer `MAJOR.MINOR.PATCH+BUILD`: the dotted part = CFBundleShortVersionString / ProductVersion (user-facing, what the appcast matches), the +N = CFBundleVersion / monotonic build (must strictly increase for macOS auto-update + notarization re-uploads). Stamp the SAME version into the Go sidecar at build for the health-handshake.**
   ```
   # frontend/pubspec.yaml
   version: 0.1.0+1   # -> macOS CFBundleShortVersionString=0.1.0, CFBundleVersion=1
   
   # Go sidecar build stamps the matching version (lockstep):
   cd backend && go build -ldflags "-X main.version=0.1.0 -X main.build=1" -o ../frontend/<platform-staging>/anselm-server ./cmd/server
   ```
   — +BUILD must be globally monotonic across the app's lifetime (Apple rejects a notarized upload whose CFBundleVersion isn't higher than a prior one for the same bundle id). The appcast/auto-update feed (desktop_updater, README line ~322) compares the dotted version; bundled sidecar version must equal the app's so /api/v1/health drift detection (README line ~383) stays quiet. CI should derive both fr
9. **Add a doc-sync row so identity stays the single source of truth. Per CLAUDE.md doc nuance #9, record the frozen identity matrix in references/frontend (architecture / platform-foundation landing) so signing, AUMID, deep-link, and data-dir sections all cite ONE table — and so nobody re-introduces a host.anselm-derived id.**
   ```
   # add to references/frontend/<platform> a single identity table:
   # | axis | value |
   # | reverse-DNS prefix | app.getanselm |
   # | macOS CFBundleIdentifier | app.getanselm.anselm |
   # | macOS sidecar codesign -i | app.getanselm.sidecar |
   # | Windows AUMID + .rc CompanyName | app.getanselm.anselm / The Anselm Project |
   # | Linux app-id / .desktop / metainfo | app.getanselm.anselm |
   # | display name | Anselm | CFBundleName | Anselm |
   # | min OS | macOS 10.15 / Windows 10 1809 / Ubuntu 20.04-class GTK3 |
   # | version scheme | SemVer MAJOR.MINOR.PATCH+BUILD, build monotonic |
   ```
   — This table is referenced by the signing section (sidecar -i), the notifications section (AUMID), the deep-links section (CFBundleURLName), and the data-dir section (leaf). One edit point prevents drift across the playbook.

**与我们(sidecar/下载运行时)**:TWO bundled-sidecar / downloaded-runtime identity rules that this section must enforce, because they bite later in signing/notarization:

(1) NESTED SIDECAR NEEDS ITS OWN UNIQUE BUNDLE ID. The Go `cmd/server` binary ships inside Contents/MacOS (or Contents/Resources) of the .app. Apple's notarization rejects a sub-component that re-uses the parent's CFBundleIdentifier or carries none. When you `codesign` the sidecar bottom-up (before the .app, no --deep), pass `codesign -i <prefix>.anselm.sidecar` so the embedded binary gets a DISTINCT-but-consistent identifier under the same reverse-DNS prefix (e.g. app.getanselm.sidecar vs app.getanselm.anselm for the app). A plain Go executable has no Inf

**工具**:`flutter_launcher_icons`（macOS (writes Assets.xca） · `path_provider`（all 3 desktop） · `package_info_plus`（all 3 desktop） · `Apple bundle-ID + Info.plist key reference (CFBundleIdentifier / CFBundleName / CFBundleDisplayName / CFBundleURLTypes / LSMinimumSystemVersion)`（macOS） · `Windows AppUserModelID (AUMID) reference`（Windows） · `Flatpak / AppStream app-id + .desktop + hicolor conventions`（Linux） · `rsms macOS distribution gist (signing/notarization/quarantine canonical reference)`（macOS）
**坑**:1) THE EXISTING `host.anselm.anselm` IS A TRAP — it both stutters and is the reverse of the BANNED anselm.host domain. Fix it before ANY signed build; after release the macOS data-dir + downloaded-runtime store path are keyed to it and become unmovable (orphans every user's DB + downloaded python/node). 2) The bundled Go sidecar MUST get its OWN unique identifier at sign time via `codesign -i <prefix>.sidecar` — a plain executable has no Info.plist, so this flag is the ONLY place its identity ex
**成本/时间**:$99/yr (Apple Developer Program, already committed) is the only hard cost touching identity. Reverse-DNS prefix: $0 if u ｜ First-time setup: ~2–4 hours — most of it is the one-time DECISION on the reverse-DNS prefix + auditing every file that 

> ⚠ **对抗复审纠正** — **命令**:Mostly accurate and current. Verified against the live repo + 2025/2026 sources:

- CONFIRMED the repo really has `host.anselm.anselm` in SOURCE (not just build artifacts): frontend/macos/Runner/Configs/AppInfo.xcconfig:11, project.pbxproj:482/497/512 (3 RunnerTests lines), frontend/windows/runner/Runner.rc:92 (Company ｜ **成本**:Costs are accurate for 2025-2026. Apple Developer Program = $99/yr USD confirmed (developer.apple.com/programs/whats-included). flutter_launcher_icons / path_provider / package_info_plus / app_links all MIT/free. Domain ~$10-12/yr (Cloudflare at-cost, Namecheap) is right; io.github.<user> route is genuinely $0 and ban- ｜ **我们特殊点**:The three sidecar/runtime rules HOLD UP and are the strongest part of the section:

(1) NESTED SIDECAR UNIQUE BUNDLE ID — CORRECT and well-grounded. Apple TN2206/codesign(1) confirm that a plain executable derives identity from pathname unless `-i` is given, and notarization tooling flags components that reuse the pare ｜ **工具纠正**:`flutter_launcher_icons`→Linux claim is correctly hedged: the package does NOT meaningfully generate Linux/hicolor ；`path_provider`→getApplicationSupportDirectory on macOS uses NSApplicationSupportDirectory and the leaf IS；`package_info_plus`→Reads CFBundleShortVersionString/CFBundleVersion etc. as claimed. Accurate.

---

## build-release-ci — Build + release orchestration and CI/CD end-to-end (cross build matrix, version single-source-of-truth, fastforge umbrella, GitHub Actions signing pipelines, mise/Makefile wiring)

> **目标**:A reproducible, push-button release flow that takes Anselm from source to SIGNED + NOTARIZED/trusted + INSTALLABLE + AUTO-UPDATING artifacts on macOS (arm64 universal DMG), Windows (x64 signed Inno installer), and Linux (x64 AppImage + deb), with the GOOS/GOARCH-matched Go sidecar built, placed in each bundle at the correct path with the exec bit, signed/notarized in lockstep, and a single version+build-number (pubspec) flowing coherently into Info.plist / Windows file version / MSIX / sidecar /version endpoint. Orchestrated by fastforge under one distribute_options.yaml, driven by per-OS GitHub Actions runners, published to GitHub Releases (ban-resistant host) which doubles as the appcast/update-feed host.

**前置(含成本)**
- **Apple Developer Program (Developer ID Application cert + App Store Connect API key for notarytool)** — _$99/yr recurring (confirmed decision)_ — Enroll at developer.apple.com; in Certificates create a 'Developer ID Application' cert (export .p12 with private key); in App Store Connect > Users and Access > Integrations > App Store Connect API create a Team key (.p8) — capture Key ID + Issuer ID. notaryt
- **Windows code-signing certificate — Azure Artifact Signing (formerly Trusted Signing)** — _~$9.99/mo recurring (Basic tier; covers identity validation _ — Create an Azure subscription, provision a Trusted/Artifact Signing account + Certificate Profile, complete identity validation. Sign via the Azure 'Trusted Signing' GitHub Action or signtool with the dlib. Short-lived (3-day) certs minted per-sign — no HSM/USB
- **EdDSA (ed25519) update-signing key pair for the appcast** — _$0_ — Generate once with Sparkle's generate_keys (macOS: stores private key in Keychain, prints SUPublicEDKey) or sign_update; for WinSparkle/desktop_updater generate an ed25519 pair. Store the PRIVATE key as a GitHub secret (SPARKLE_ED_PRIVATE_KEY); embed the PUBLI
- **GitHub repo + Actions minutes (CI runners — incl. a real macOS runner for Apple signing/notarization)** — _Free tier: 2,000 included minutes/mo (Pro: 3,000); beyond th_ — GitHub repo already exists. macOS signing+notarization REQUIRES a GitHub-hosted macOS runner (macos-14/macos-15, Apple-silicon) — Linux/Windows runners cannot codesign/notarize Apple artifacts.
- **fastforge CLI (umbrella packager, ex flutter_distributor)** — _$0 (MIT)_ — dart pub global activate fastforge (current v0.6.8, actively maintained, Rust rewrite underway). Pin in mise/CI. Provides macos dmg/pkg, windows exe(Inno)/msix, linux appimage/deb/rpm from one distribute_options.yaml.
- **Platform packaging tools fastforge shells out to** — _$0_ — macOS: appdmg/create-dmg + Xcode CLT (codesign, notarytool, stapler) on the macOS runner. Windows: Inno Setup 6 (innosetup) installed on the windows runner via choco/winget; msix via the msix pub package. Linux: appimagetool + dpkg-deb (apt) on the linux runne

**步骤**
1. **Wire mise + Makefile as the single toolchain entry. Add fastforge + platform packagers to the toolchain and add release Make targets next to the existing `build`/`verify`. mise already pins go=1.25, flutter=3.41.9 — keep CI on the SAME pins so local and CI builds are byte-comparable.**
   ```
   # mise.toml — add fastforge as a pinned tool\n[tools]\ngo = \"1.25\"\nflutter = \"3.41.9\"\n\"dart:fastforge\" = \"0.6.8\"\n\n# Makefile — replace the host-only `build` with a sidecar matrix target\nSIDECAR_OUT := frontend/sidecar\nsidecar-%:  ## sidecar-darwin-arm64 etc.\n\t@os=$(word 2,$(subst -, ,$*)); arch=$(word 3,$(subst -, ,$*)); \\\n\t cd backend && GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(RUN) go build \\\n\t   -ldflags \"-X main.version=$(shell grep '^version:' frontend/pubspec.yaml | cut -d' ' -f2) -s -w\" \\\n\t   -o ../$(SIDECAR_OUT)/anselm-server-$$os-$$arch ./cmd/server
   ```
   — CGO_ENABLED=0 is mandatory — ADR 0001's glebarez/go-sqlite is pure-Go, so cross-compile is plain GOOS/GOARCH with zero platform deps. The -X main.version ldflag stamps the SAME pubspec version into the sidecar so the /version endpoint matches the client (closes the version-skew gap the catalog flags as P2).
2. **Establish pubspec.yaml `version:` (currently 0.1.0+1) as the ONE source of truth and make every platform derive from it. Flutter 3.3+ already propagates pubspec version to macOS Info.plist (CFBundleShortVersionString=build-name, CFBundleVersion=build-number) AND Windows Runner.rc VERSION_AS_NUMBER/VERSION_AS_STRING automatically — do NOT hand-edit those. For MSIX, mirror via msix_config.msix_version (needs 4-part a.b.c.d). The sidecar gets it via the -X ldflag from step 1. In CI, never pass literal versions — read pubspec.**
   ```
   # CI: derive once, pass to flutter + fastforge\nVER=$(grep '^version:' frontend/pubspec.yaml | sed 's/version: //')\nBUILD_NAME=${VER%%+*}; BUILD_NUM=${VER##*+}\nflutter build macos --release --build-name=$BUILD_NAME --build-number=$BUILD_NUM\n# msix_config in pubspec.yaml (Windows store-version coherence):\n#   msix_config:\n#     msix_version: 0.1.0.1   # a.b.c.d — bump per release
   ```
   — Gotcha: flutter only reads pubspec version at build time; bumping pubspec is the single edit per release. Windows file version's 4th field = build-number. MSIX requires a STRICTLY increasing 4-part version if you ever ship via App Installer — but the catalog correctly notes Anselm ships Inno (full-trust, needed so the AppContainer doesn't block the sidecar+directInstaller), so MSIX ordering is a n
3. **Place the matched sidecar into each platform bundle at the correct path with exec bit, BEFORE packaging. macOS: Contents/MacOS/ (alongside the app exec, inside Hardened Runtime scope). Windows: next to the .exe in the install dir. Linux: in the AppDir next to the bundle. Do this in a flutter build post-step / fastforge pre-package hook; the Xcode Copy Files Build Phase is the most robust for macOS notarization scope.**
   ```
   # macOS — after `flutter build macos`, embed + exec bit (then sign in step 6)\nAPP=build/macos/Build/Products/Release/anselm.app\ncp frontend/sidecar/anselm-server-darwin-arm64 \"$APP/Contents/MacOS/anselm-server\"\nchmod +x \"$APP/Contents/MacOS/anselm-server\"\n# Windows — copy beside Runner.exe before Inno packaging\ncp frontend/sidecar/anselm-server-windows-amd64.exe build/windows/x64/runner/Release/anselm-server.exe\n# Linux — copy into bundle dir before appimage packaging\ncp frontend/sidecar/anselm-server-linux-amd64 build/linux/x64/release/bundle/anselm-server && chmod +x build/linux/x64/release/bundle/anselm-server
   ```
   — The Dart side locates it via f(Platform.resolvedExecutable) + per-OS switch (catalog row). For macOS universal you may lipo two arch builds: `lipo -create -output anselm-server-universal arm64 amd64`. The nested macOS binary MUST be inside the signed/notarized tree (Contents/MacOS) — Resources also works but MacOS is conventional for executables.
4. **Author distribute_options.yaml — the single fastforge release manifest covering all three OSes. One `releases.production.jobs[]` entry per artifact; build_args carry build-name/build-number derived from pubspec.**
   ```
   # frontend/distribute_options.yaml\noutput: build/dist\nreleases:\n  production:\n    jobs:\n      - name: macos-dmg\n        package: { platform: macos, target: dmg }\n      - name: windows-inno\n        package: { platform: windows, target: exe }   # exe target = Inno Setup\n      - name: windows-msix          # optional secondary\n        package: { platform: windows, target: msix }\n      - name: linux-appimage\n        package: { platform: linux, target: appimage }\n      - name: linux-deb\n        package: { platform: linux, target: deb }\n# run on each OS runner (only its own targets build there):\n# fastforge release --name production
   ```
   — fastforge cannot cross-build packages — each OS's jobs run only on that OS's runner (macOS dmg on macos-14, etc.). fastforge build_args pass through to `flutter build`. Signing is NOT done by fastforge for macOS/Windows — keep it explicit (steps 6-7) so you control bottom-up signing + entitlements; fastforge owns only build+package+(optionally)publish.
5. **Stand up the GitHub Actions release workflow with a per-OS matrix triggered on a version tag. Each leg: checkout, setup mise (pins go+flutter+fastforge), build sidecar for its arch, flutter build, embed sidecar, sign, package via fastforge, upload artifact. A final job creates the GitHub Release + regenerates the appcast.**
   ```
   # .github/workflows/release.yml (skeleton)\non: { push: { tags: ['v*'] } }\njobs:\n  build:\n    strategy:\n      matrix:\n        include:\n          - { os: macos-14,      gooses: darwin,  goarch: arm64 }\n          - { os: windows-latest, gooses: windows, goarch: amd64 }\n          - { os: ubuntu-latest,  gooses: linux,   goarch: amd64 }\n    runs-on: ${{ matrix.os }}\n    steps:\n      - uses: actions/checkout@v4\n      - uses: jdx/mise-action@v2        # installs go+flutter+fastforge from mise.toml\n      - run: make sidecar-${{ matrix.gooses }}-${{ matrix.goarch }}\n      - run: flutter build ${{ matrix.gooses == 'darwin' && 'macos' || matrix.gooses }} --release\n      # ... embed sidecar (step 3), sign (steps 6/7), then:\n      - run: dart pub global run fastforge release --name production\n      - uses: actions/upload-artifact@v4\n        with: { name: dist-${{ matrix.os }}, path: build/dist/** }
   ```
   — macOS leg MUST be a macos-* runner (Apple tooling). Use jdx/mise-action so CI uses the EXACT pinned go/flutter/fastforge as local — reproducibility. Keep signing secrets out of forks: gate on github.repository_owner.
6. **macOS signing leg: import Developer ID cert from a base64 secret into a temporary keychain, sign BOTTOM-UP (nested sidecar first with Hardened Runtime + the runtime entitlements, then helpers, then the .app — NEVER --deep), package DMG via fastforge, notarize with notarytool (API key from secrets), staple. Hardened Runtime is required because the app spawns downloaded runtimes (ADR 0001).**
   ```
   # import cert\necho \"$MACOS_CERT_P12_BASE64\" | base64 --decode > cert.p12\nsecurity create-keychain -p \"$KCPW\" build.keychain\nsecurity import cert.p12 -k build.keychain -P \"$MACOS_CERT_PWD\" -T /usr/bin/codesign\nsecurity set-key-partition-list -S apple-tool:,apple: -s -k \"$KCPW\" build.keychain\nID=\"Developer ID Application: NAME (TEAMID)\"\n# bottom-up: sidecar with runtime entitlements\ncodesign -f -o runtime --timestamp --entitlements sidecar.entitlements -s \"$ID\" \"$APP/Contents/MacOS/anselm-server\"\ncodesign -f -o runtime --timestamp --entitlements Runner-Release.entitlements -s \"$ID\" \"$APP\"\n# package + notarize + staple\nfastforge release --name production   # builds the .dmg\nxcrun notarytool submit build/dist/*/anselm-*.dmg --key AuthKey.p8 --key-id \"$KID\" --issuer \"$ISS\" --wait\nxcrun stapler staple build/dist/*/anselm-*.dmg
   ```
   — Sign the OUTER app AFTER the nested binary (bottom-up); --deep is forbidden (mis-signs nested entitlements). Entitlements set (Runner-Release.entitlements) for spawning downloaded/JIT runtimes: com.apple.security.cs.disable-library-validation=true, allow-jit=true, allow-unsigned-executable-memory=true, allow-dyld-environment-variables=true; App Sandbox OFF. Notarytool waits inline (~2-15 min). Sta
7. **Windows signing leg: sign the bundled sidecar exe + the Flutter exe + the final Inno installer with Azure Trusted/Artifact Signing (timestamped). Sign the sidecar BEFORE packaging, the installer AFTER fastforge builds it. Linux: AppImage needs no signing trust chain (optional GPG/sha256 in the appcast).**
   ```
   # Windows — Azure Trusted Signing GitHub Action (signs exe + installer)\n- uses: azure/trusted-signing-action@v0.5\n  with:\n    azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}\n    azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}\n    azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}\n    endpoint: https://eus.codesigning.azure.net/\n    trusted-signing-account-name: anselm\n    certificate-profile-name: anselm-cp\n    files-folder: build/windows/x64/runner/Release\n    files-folder-filter: exe\n# then fastforge builds the Inno installer; sign the installer .exe the same way
   ```
   — Sign sidecar+app exe BEFORE Inno packs them, then sign the installer itself — SmartScreen flags any unsigned binary in the update path. Azure short-lived (3-day) certs are minted per-sign, ideal for CI (no HSM token). Timestamping (--timestamp / RFC3161) means signatures outlive the cert. Inno (full-trust) chosen over MSIX so the AppContainer doesn't block the sidecar child-process + directInstall
8. **Publish to GitHub Releases (ban-resistant host) and generate the signed appcast/update-feed there. GitHub Releases serves both the artifacts AND the appcast.xml/update-index — no custom domain (the banned anselm.host is avoided). Sparkle's generate_appcast EdDSA-signs each entry; SUFeedURL points at the raw GitHub Releases asset URL. Sidecar updates in lockstep because it lives inside the bundle.**
   ```
   # final 'release' job (after all matrix artifacts uploaded)\n- uses: actions/download-artifact@v4\n- name: generate signed appcast (macOS)\n  run: ./bin/generate_appcast --ed-key-file <(echo \"$SPARKLE_ED_PRIVATE_KEY\") build/dist\n- uses: softprops/action-gh-release@v2\n  with:\n    files: |\n      build/dist/**/*.dmg\n      build/dist/**/*.exe\n      build/dist/**/*.AppImage\n      build/dist/**/*.deb\n      appcast.xml\n# App reads SUFeedURL = https://github.com/<org>/anselm/releases/latest/download/appcast.xml
   ```
   — GitHub Releases is the ban-resistant host (no custom domain, no DNS to block); fastforge also has a github publish_to target. The appcast carries per-release notes + EdDSA signature + download URL. Because the whole bundle (incl. sidecar) is the update unit, no separate sidecar feed is needed — lockstep guaranteed. Linux has no blessed in-app updater: desktop_updater (verified-zip full replace) re

**与我们(sidecar/下载运行时)**:TWO architecture facts dominate this section. (1) BUNDLED Go sidecar: every release artifact embeds cmd/server built for its exact GOOS/GOARCH (CGO_ENABLED=0, pure-Go sqlite per ADR 0001). The build matrix is therefore DOUBLE — flutter build {os}x{arch} AND a matched `go build`, with the binary copied into the bundle (macOS Contents/MacOS, Windows beside .exe, Linux AppDir) + exec bit set BEFORE packaging. On macOS the nested binary must be signed FIRST and bottom-up (codesign -o runtime --timestamp --entitlements sidecar.entitlements, NO --deep) so it falls inside the notarized tree; on Windows it must be Authenticode-signed before Inno packs it or SmartScreen flags the update. The sidecar 

**工具**:`fastforge (ex flutter_distributor / leanflutter)`（macOS dmg/pkg, Windows e） · `fastlane`（macOS/iOS-centric (match） · `jdx/mise-action (CI toolchain)`（Linux/macOS/Windows runn） · `Apple-Actions/import-codesign-certs + xcrun notarytool/stapler`（macOS runner only） · `azure/trusted-signing-action (Azure Artifact/Trusted Signing)`（Windows runner） · `Sparkle generate_appcast + auto_updater (macOS), WinSparkle, desktop_updater (Linux)`（macOS+Windows (Sparkle/W） · `softprops/action-gh-release + GitHub Releases`（host-agnostic (publishes） · `Go toolchain cross-compile + Inno Setup 6 + appimagetool`（all 3 (Go cross-compiles）
**坑**:1) macOS signing/notarization is IMPOSSIBLE off a macOS runner — the matrix MUST include macos-14/15; Linux can't notarize Apple artifacts. 2) Sign BOTTOM-UP and NEVER use codesign --deep — it mis-applies the app's entitlements to the nested sidecar and breaks the runtime entitlements; sign the sidecar with sidecar.entitlements FIRST, then the .app. 3) Hardened Runtime without disable-library-validation/allow-jit/allow-unsigned-executable-memory/allow-dyld-environment-variables will let the app 
**成本/时间**:Recurring: $99/yr Apple Developer + ~$120/yr ($9.99/mo) Azure Artifact Signing = ~$219/yr mandatory signing identity. Ed ｜ First-time setup: 3–5 focused days — Apple enrollment + cert/API-key dance (~0.5d, plus up-to-48h Apple identity validat

> ⚠ **对抗复审纠正** — **命令**:Mostly correct and current; a few precise fixes.

1) VERIFIED LOCAL: mise.toml pins go=1.25, flutter=3.41.9; pubspec version: 0.1.0+1, name: anselm; Makefile has build/verify/server. So the "single edit per release = bump pubspec" and the sidecar-matrix wiring are grounded in real repo state.

2) fastforge tool name `" ｜ **成本**:Costs are accurate for 2025-2026, with one figure to update.

- Apple Developer Program $99/yr: correct.
- GitHub Actions per-minute: EXACTLY correct for the Jan 1 2026 price cut — Linux $0.006, Windows $0.010, macOS $0.062 (these now fold in the $0.002/min platform charge). The ~$1.76/release tri-OS estimate is sound. ｜ **我们特殊点**:The two architecture-specific mechanisms hold up and are, in fact, the strongest part of the section — but with two concrete risks.

CONFIRMED SOUND:
1) Bottom-up signing of the nested Go sidecar with its OWN entitlements file FIRST, then the outer .app, and NEVER --deep — this is exactly Apple's required approach (--d ｜ **工具纠正**:`fastforge (ex flutter_distribu`→pub.dev lists Windows targets as 'EXE, MSIX' and macOS as 'DMG, PKG', Linux 'AppImage, DEB；`jdx/mise-action`→Works, but see commandsAccurate #2 — the action installs from mise.toml, so if `dart:fastf；`Apple-Actions/import-codesign-`→Correct. Add: Notary API requires a TEAM key with Developer role (Personal keys rejected).

---

## Legal / licensing / privacy for shipping Anselm (open-source attribution, in-app + in-repo NOTICES, privacy policy + EULA, US export-control self-classification, trademark)

> **目标**:Every external release ships with legally-complete attribution (in-app license page + in-repo THIRD_PARTY_NOTICES) covering bundled fonts, all Dart/pub deps, all Go deps, and a correct treatment of the runtime-downloaded language runtimes; plus a truthful Privacy Policy, an EULA with AI/code-execution disclaimers, a defensible US export-control posture, and a chosen distributable name — so we can hand the app to outside users without infringement, false-advertising, or export liability. These are documents/files to produce, not abstractions.

**前置(含成本)**
- **A real legal-entity name + jurisdiction for the EULA/Privacy Policy 'we/us' and copyright line (sole-proprietor name is fine for v1; you, @weilin)** — _$0 (use your own name) or ~$0-800 one-time if you form an LL_ — Decide now: ship as an individual developer under your name, or stand up an LLC. v1 can ship under an individual name; the EULA/copyright just needs a consistent legal identity.
- **go-licenses (Google) installed in the build env to harvest Go sidecar dependency licenses** — _$0 (Apache-2.0 OSS)_ — go install github.com/google/go-licenses@latest — pin a version in mise.toml / Makefile per S22
- **A trademark clearance search for the name 'Anselm' before public launch (USPTO TESS + common-law web search); optional registration** — _$0 self-search; ~$250-350/class USPTO filing fee if you regi_ — Search tmsearch.uspto.gov for 'Anselm' in classes 9 (software) and 42 (SaaS/software services); web/app-store search for collisions. Registration optional for v1.
- **One-time decision: open-source the repo publicly or keep it closed (drives whether the EAR 'publicly available' carve-out applies)** — _$0_ — Decide before first external release. Public OSS → EAR 740.13(e) publicly-available path (one email, then not subject to EAR). Closed-source → mass-market 5D992.c self-classification path.

**步骤**
1. **Inventory what we actually SHIP vs what is DOWNLOADED. Shipped-in-package (attribution attaches): MiSansVF.ttf, JetBrainsMono.ttf, every Dart/pub dependency compiled into the Flutter app, the Lucide icon font, Material icons, and the bundled Go sidecar (cmd/server) with all its Go module dependencies. NOT shipped (downloaded at first use by directInstaller, ADR 0001): python-build-standalone, node, uv, dotnet — we never redistribute these bytes, the user downloads them from the upstream vendor, so their redistribution/attribution obligations do NOT attach to our package. This download-vs-bundle distinction is the single most important licensing call in this section.**
   — Source: ADR 0001 confirms runtimes are streamed from upstream (astral pbs, nodejs.org, astral uv, builds.dotnet.microsoft.com) at first use, not embedded. Because we distribute zero runtime bytes, we owe no NOTICE for them — but see step 7 for the in-app disclosure we still owe.
2. **Handle the MiSans font obligation — this is our HIGHEST-RISK attribution item. Xiaomi's 'MiSans Font Intellectual Property License Agreement' grants a royalty-free, non-exclusive, REVOCABLE worldwide license; free for commercial use; embedding the font inside an App is EXPLICITLY permitted (the no-redistribution clause exempts 'applications (Apps)' you create). BUT two hard obligations: (a) you MUST 'specifically note in the software that MiSans Fonts was used' — mandatory in-app credit, not optional; (b) you may NOT let users extract/redistribute the raw MiSansVF.ttf as a standalone file. Action: download the official PDF license, save it verbatim into the repo, and add a MiSans credit line to the in-app About/License page.**
   ```
   curl -L 'https://hyperos.mi.com/font-download/MiSans%E5%AD%97%E4%BD%93%E7%9F%A5%E8%AF%86%E4%BA%A7%E6%9D%83%E8%AE%B8%E5%8F%AF%E5%8D%8F%E8%AE%AE.pdf' -o frontend/assets/fonts/MiSans-License.pdf
   ```
   — Confirmed from Xiaomi license text: 'royalty-free... users should specifically note in the software that MiSans Fonts was used' and 'shall not... redistribute... However, this restriction does not apply to any other work... such as... applications (Apps).' Revocability is a residual product risk — note it in an ADR; if Xiaomi ever revokes, you swap the UI face. Do NOT rely on the third-party dsrka
3. **Handle JetBrains Mono — SIL Open Font License 1.1. The OFL.txt (JetBrainsMono-OFL.txt) is ALREADY in assets/fonts/. OFL obligations: (a) keep the copyright + license notice with the font (done), (b) the font cannot be sold by itself (we don't), (c) the 'JetBrains Mono' Reserved Font Name cannot be used by a MODIFIED version (we ship it unmodified, so fine), (d) the font must be bundled with software, not sold standalone (fine). No further file needed beyond keeping OFL.txt shipped — but it must appear in the in-app license page (step 4) because Flutter does NOT auto-collect asset licenses.**
   — OFL 1.1 is the standard libre-font license. Our only live risk would be renaming/modifying the .ttf while keeping the name 'JetBrains Mono' — we don't. Keep JetBrainsMono-OFL.txt in the package.
4. **Wire BOTH bundled fonts into Flutter's LicenseRegistry — because Flutter's showLicensePage() auto-aggregates only each pub PACKAGE's root LICENSE file, NOT asset files like fonts. Add a LicenseRegistry.addLicense() callback in bootstrap that loads the MiSans license + JetBrains Mono OFL from rootBundle and registers them under named entries. This makes the in-app 'Licenses' page (the P0 'About/Help/Licenses' item already in the catalog) legally complete for fonts.**
   ```
   // in bootstrap.dart, before runApp:
   LicenseRegistry.addLicense(() async* {
     yield LicenseEntryWithLineBreaks(['MiSans'], await rootBundle.loadString('assets/fonts/MiSans-License.txt'));
     yield LicenseEntryWithLineBreaks(['JetBrains Mono'], await rootBundle.loadString('assets/fonts/JetBrainsMono-OFL.txt'));
   });
   ```
   — Confirmed: 'Custom font assets... are not automatically included in showLicensePage(). You must explicitly use LicenseRegistry.addLicense().' Flutter DOES auto-aggregate pub deps' root LICENSE files into the bundled LICENSE asset, so all Dart/pub deps (MIT/BSD/Apache) are covered by showLicensePage for free. Convert the MiSans PDF terms to a plain-text MiSans-License.txt for in-app display (PDF ca
5. **Generate the Go sidecar's THIRD_PARTY_NOTICES with go-licenses. flutter's LicenseRegistry covers the Dart side but knows NOTHING about the bundled Go binary's dependencies (glebarez/go-sqlite, cel-go, any HTTP/crypto libs). Run go-licenses against cmd/server to (a) save every dependency's license text + copyright into a directory for redistribution compliance and (b) produce a CSV report to eyeball for any copyleft surprises. Commit the result as backend/THIRD_PARTY_NOTICES/ and surface it in-app (ship it as a Flutter asset or have the sidecar expose it at /api/v1/licenses).**
   ```
   go-licenses save ./cmd/server --save_path=backend/THIRD_PARTY_NOTICES --force
   go-licenses report ./cmd/server --template=notices.tpl > backend/THIRD_PARTY_NOTICES.txt
   go-licenses check ./cmd/server --disallowed_types=forbidden,restricted
   ```
   — go-licenses (github.com/google/go-licenses, Apache-2.0) 'collects all of the license documents, copyright notices and source code into a directory in order to comply with license terms on redistribution.' The `check` subcommand fails the build if any forbidden/restricted (e.g. GPL/AGPL) license sneaks in — wire it into `make verify` as a gate. Pure-Go sqlite (glebarez) is MIT/Apache-friendly; veri
6. **Produce the in-repo THIRD_PARTY_NOTICES master + the in-app aggregation. Files to create: (1) THIRD_PARTY_NOTICES.md at repo root = human index pointing at the font licenses, the Flutter-aggregated pub LICENSE, and backend/THIRD_PARTY_NOTICES.txt; (2) the in-app 'Licenses' page already aggregates pub + fonts via LicenseRegistry; (3) bundle the Go notices text as a Flutter asset and append it to the LicenseRegistry too so the ONE in-app page is complete across all three dependency worlds (Dart, Go, fonts). Per CLAUDE.md doc-discipline, regenerate these in the SAME commit whenever deps change (add a `make notices` target).**
   ```
   // extend the bootstrap callback:
   yield LicenseEntryWithLineBreaks(['Go sidecar (cmd/server)'], await rootBundle.loadString('assets/legal/go-third-party-notices.txt'));
   ```
   — This satisfies the catalog's P0 'Open-source licenses / acknowledgements page' end-to-end. Make `make notices` = (go-licenses save+report) + copy go notices into frontend/assets/legal/ — deterministic, committed, gated in fe-verify/verify.
7. **Write the PRIVACY POLICY (PRIVACY.md + in-app + hosted URL). For a local-first single-user app this is short and TRUTHFUL: (a) all workspace data (entities, runs, SQLite, blobs) stays on the user's machine; (b) the app makes NO telemetry/analytics calls by default; (c) the ONLY outbound network is (i) user-configured model/provider API calls — the user's prompts/data go to whatever LLM provider THEY configured, governed by THAT provider's policy — and (ii) on first use, downloading language runtimes from upstream vendors (python.org/astral/nodejs/microsoft) and update checks (step 11); (d) any opt-in crash reporting (Sentry, off by default per catalog) is named. Model it on the AnythingLLM desktop privacy policy (same local-first AI-desktop shape).**
   — AnythingLLM's desktop privacy policy is the canonical real-world template for this exact product shape (local data + user-configured model calls). Even with no telemetry, ship a privacy policy: it converts a strong factual claim ('nothing leaves your machine except X') into a binding promise users trust. Disclose the runtime downloads + update pings explicitly — they ARE outbound connections.
8. **Write the EULA / Terms (EULA.md + first-run accept + in-app). Must contain, given our architecture: (a) license grant + restrictions (no reselling the binary, no extracting bundled fonts — ties back to MiSans/OFL); (b) 'AS IS', no-warranty, limitation-of-liability (standard for free software); (c) a CODE-EXECUTION / SANDBOX disclaimer — Anselm downloads and EXECUTES third-party language runtimes and runs user/AI-authored Function/Handler/MCP code on the user's machine; user assumes responsibility for code they run and accepts that the sandbox is isolation-best-effort, not a security guarantee; (d) an AI-OUTPUT disclaimer — model outputs may be wrong/harmful, user is responsible for reviewing tool actions (ties to the danger-approval gate); (e) third-party-runtime + third-party-model-provider terms flow-through (user must comply with python/node/dotnet and their LLM provider's terms); (f) export-compliance clause (user won't use it in embargoed jurisdictions).**
   — The code-execution + AI-output disclaimers are NON-optional for us specifically — a generic app EULA omits them. They are the legal mirror of ADR 0001 (download+execute runtimes) and the tool-danger-approval design. 'Even free apps should use a EULA... to control how people use it, protect yourself, reduce legal dispute' (industry guidance).
9. **US export-control self-classification — pick the path based on the open-source decision (prerequisite 4). Anselm ships crypto: TLS to providers, AES-GCM at-rest encryption of API keys (backend), update-signature verification (ed25519). PATH A (repo is PUBLIC OSS): the source is 'publicly available' — under EAR 740.13(e) send a ONE-TIME email notification of the source-code URL to crypto@bis.doc.gov and enc@nsa.gov; thereafter the publicly-available source AND its corresponding object code are NOT subject to the EAR. PATH B (closed-source): the app is mass-market software whose crypto is standard/ancillary → self-classify as ECCN 5D992.c. Mass-market 5D992.c items are EXEMPT from the annual Feb-1 self-classification report (the annual report applies to 740.17(b)(1) ENC items, explicitly excepting mass-market 5A992.c/5D992.c). Document the chosen classification in an ADR.**
   ```
   # Path A one-time email (template):
   # To: crypto@bis.doc.gov, enc@nsa.gov
   # Subject: 740.13(e) notification - publicly available encryption source code
   # Body: URL of the public repo, brief description, your name/contact
   ```
   — Confirmed: annual self-classification report is required for License Exception ENC 740.17(b)(1) items 'except for mass market Encryption Items... classified as ECCN 5A992.c... or 5D992.c.' So a mass-market desktop app using standard TLS/AES does NOT owe the annual CSV report. If PUBLIC OSS, the 740.13(e) email is the only filing and then it's out of the EAR entirely. Neither path involves fees. Ke
10. **Trademark / name. Do a clearance search for 'Anselm' in software classes (USPTO class 9 software, class 42 software services) plus app-store/web search BEFORE public launch, because the name appears on the binary, in stores, and in the appcast. If clear, optionally register; if a strong conflict exists, rename now (cheap pre-launch, expensive post-launch). Set the copyright/legal line consistently across: macOS Info.plist (NSHumanReadableCopyright), Windows version resource, the About box, EULA, and the LICENSE header.**
   ```
   # self-search first (free): tmsearch.uspto.gov -> 'Anselm', classes 009 + 042
   ```
   — 'Anselm' is also a common given name / philosopher (Anselm of Canterbury) — likely low collision in software TM, but verify. Registration ~$250-350/class via TEAS is optional for v1; the clearance search is the must-do (avoid building brand equity on an infringing name).
11. **Disclose the auto-update + appcast connection in the privacy policy and make update artifacts signature-verified (ties to the ban-resistant GitHub-Releases hosting decision). The update check is an outbound connection to the update host (GitHub Releases) — name it in PRIVACY.md. The signed appcast (ed25519, per the auto-update catalog row) is also an export-control 'crypto' touchpoint already covered by step 9's classification.**
   — Because the prior domain was banned, GitHub Releases as the appcast/update host keeps this ban-resistant and means the 'outbound update connection' disclosure points at github.com, not a custom domain. No new legal doc — one sentence in PRIVACY.md + the existing signing work.

**与我们(sidecar/下载运行时)**:BUNDLED GO SIDECAR: it is shipped inside every platform package, so its Go-module dependency licenses MUST be harvested and redistributed — Flutter's LicenseRegistry/showLicensePage is blind to it. Run `go-licenses save ./cmd/server` to collect license texts + `go-licenses check --disallowed_types=forbidden,restricted` as a build gate, then fold the resulting notices into the single in-app license page (as a bundled asset registered via LicenseRegistry.addLicense). Verify cel-go (Apache-2.0, needs NOTICE) and glebarez/go-sqlite. The sidecar's own crypto (AES-GCM key encryption, ed25519 update verify, TLS egress) is part of the export-control classification in step 9 — classify the app+sideca

**工具**:`go-licenses (Google)`（Any (Go toolchain) — run） · `Flutter LicenseRegistry + showLicensePage (SDK built-in)`（macOS/Windows/Linux (Flu） · `USPTO TESS / tmsearch.uspto.gov`（Web） · `BIS encryption self-classification / 740.13(e) notification`（Email filing）
**坑**:1) MiSans license is REVOCABLE and Chinese-law-governed — if Xiaomi ever revokes, we must swap the UI font; record this residual risk in an ADR rather than assuming it's permanent. 2) Do NOT cite the third-party dsrkafuu/misans 'Apache-2.0' repo as our basis — we bundle Xiaomi's official MiSansVF.ttf under Xiaomi's own IP License, not that re-subset; using the wrong license text is a real compliance error. 3) Flutter's auto-license collection is a trap: it silently omits asset fonts AND the enti
**成本/时间**:$0 mandatory: go-licenses (free OSS), Flutter SDK (free), USPTO self-search (free), BIS 740.13(e) email or 5D992.c self- ｜ First-time setup ~1.5-2.5 days of focused work: ~0.5 day wiring go-licenses + LicenseRegistry + `make notices` target; ~

> ⚠ **对抗复审纠正** — **命令**:Mostly correct, with two notable issues and one stale install path.

1) go-licenses install path is STALE. Prerequisite says `go install github.com/google/go-licenses@latest` and the tools list says "v1.6.0 tagged, v2 in progress." Reality (verified on the repo): go-licenses shipped v2.0.1 (Sept 2025) and the documente ｜ **成本**:One real error, rest fine.

WRONG: USPTO trademark registration cost. Section says '~$250-350/class' (appears in prerequisite, step 10, tool note, and cost summary). As of Jan 18 2025 the USPTO consolidated TEAS Plus ($250) and TEAS Standard ($350) into a SINGLE base application of $350/class — the $250 tier no longer  ｜ **我们特殊点**:The Anselm-specific reasoning is the strongest part of the section and largely holds up.

SOLID: (1) The download-vs-bundle distinction is legally sound — bytes you never redistribute carry no redistribution/attribution obligation; you owe disclosure (privacy) + flow-through (EULA), not a NOTICE. Correct and well-flagg ｜ **工具纠正**:`go-licenses (Google)`→Real, Apache-2.0, actively maintained — but section says 'v1.6.0 tagged, v2 in progress.' ；`Flutter LicenseRegistry + show`→First-party Flutter SDK. Verified: pub package root LICENSE files auto-aggregated; asset f；`USPTO TESS / tmsearch.uspto.go`→TESS was RETIRED Nov 30 2023 and replaced by the cloud 'Trademark Search' system. The URL 

---

## 发行就绪:关键路径 + 仍缺项

**关键路径(从源码到首个外部可发行版,按依赖排序)**:

FIRST RESOLVE IDENTITY (blocks everything): pick a final, reverse-DNS-clean, non-banned bundle/app identity (replace host.anselm.anselm — it both double-stutters and reverses to the banned anselm.host); generate all per-OS icons from one source; declare min-OS + version/build scheme tied to pubspec; register anselm://. 2) PROCURE IN PARALLEL, by lead time: (a) Apple Developer Program — enroll INDIVIDUAL ($99, 1-3 day approval) unless you specifically need an org publisher name; (b) Windows code-signing — start NOW because SmartScreen reputation takes weeks of real downloads to warm up: use Azure Trusted/Artifact Signing if eligible (US/Canada individual, or org ≥3yrs old) else a cloud-HSM OV cert (~$200-300/yr) — do NOT pay for EV (no longer bypasses SmartScreen since 2024); (c) Linux — generate a GPG signing key (free, instant). 3) LINUX FIRST SIGNED ARTIFACT (fastest, zero gatekeeper): flutter build linux → AppImage with the per-arch Go sidecar embedded, built against a FUSE-robust runtime (handle Ubuntu 24.04 FUSE3/libfuse2t64; document --appimage-extract-and-run), + .desktop + icon + AppStream MetaInfo, GPG-sign + SHA256SUMS, zsync update-info pointing at GitHub Releases. Ship this to prove the pipeline. 4) MACOS SIGNED+NOTARIZED ARTIFACT: set entitlements on the non-sandboxed Runner = app-sandbox FALSE + cs.disable-library-validation TRUE + cs.allow-unsigned-executable-memory + cs.allow-jit (this is what lets the app spawn downloaded python/node/uv without SIGKILL; no quarantine issue since Go-downloaded files get no quarantine xattr) + SUPublicEDKey for Sparkle; sign INSIDE-OUT (Go sidecar → Flutter.framework + Sparkle.framework/XPC → app last), each with --options runtime --timestamp under Developer ID Application (NOT --deep); build DMG; notarize with notarytool using an App Store Connect API key (.p8); staple; verify with spctl. 5) WINDOWS SIGNED INSTALLER: flutter build windows → co-sign Runner.exe + bundled server.exe (Authenticode SHA256 + RFC3161 timest


**可安全推后(不阻塞首发)**:Defer (does NOT block a first external release): (1) Windows MSIX entirely — the signed per-machine Inno Setup .exe is the right primary; MSIX only matters for Store distribution, skip it. (2) Secondary Linux .deb/.rpm — AppImage covers external users; build distro-native packages only on request. (3) Flatpak/Snap — explicitly deprioritized, fine. (4) Binary DELTA updates (Sparkle deltas / zsync deltas as an optimization) — ship full-artifact replacement first; deltas are a bandwidth optimization, add later. (5) Intel+AppleSilicon UNIVERSAL binary niceties — you can ship arm64-only first if your audience is modern Macs, or a universal DMG when ready; not a trust blocker. (6) A SECONDARY/mirr


**完整性闸揪出的仍缺项 / 必须一并做的(12):**

- **macOS: exact Hardened Runtime entitlement set for spawning DOWNLOADED (non-bundled) runtimes — the real SIGKILL fix** — The macos-sign-notarize section says downloaded runtimes must 'run without being SIGKILLed' but does not name the mechanism. Confirmed: the parent .app MUST carry com.apple.security.cs.disable-library
- **macOS: inside-out signing order + --options runtime on EVERY Mach-O including the nested Go sidecar** — Flutter's .app contains Runner + Flutter.framework + dylibs + your nested cmd/server. codesign --deep is documented-but-discouraged by Apple and routinely produces notarization rejects ('the executabl
- **Windows: Azure Trusted Signing 3-year-business eligibility trap + EV no longer bypasses SmartScreen** — The windows-sign-installer section weighs 'Azure Artifact Signing vs EV' but must encode two 2024/2025 facts: (1) Azure Trusted/Artifact Signing public-trust certs require an ORG that has been a legal
- **Windows: localhost sidecar must bind 127.0.0.1 to avoid the Defender Firewall inbound prompt** — Anselm's whole model is a localhost HTTP+SSE sidecar. If the Go server binds :PORT or 0.0.0.0, every external user gets a Windows Defender Firewall 'allow access' UAC-style prompt on first run — a tru
- **Sparkle/auto_updater EdDSA keypair generation + private-key custody in CI (separate from Developer ID)** — auto-update relies on a SECOND, independent signing key (ed25519) beyond Apple/Authenticode: generate_keys creates SUPublicEDKey (embedded in Info.plist, baked at build) + a private key. The appcast e
- **Auto-update must re-deliver a fully notarized+stapled DMG/app; in-place swap of a Developer-ID app** — The appcast section says the macOS .app 'stays Developer-ID-signed + notarized through the swap' but should make explicit: Sparkle replaces the whole .app atomically, so the UPDATE ARTIFACT itself (th
- **Linux: AppImage FUSE2-vs-FUSE3 breakage on Ubuntu 24.04 + mandatory --appimage-extract-and-run / static fuse fallback** — Ubuntu 24.04 (the most common target) ships FUSE3 and renamed libfuse2→libfuse2t64; classic type-2 AppImages built against libfuse2 fail to self-mount out of the box ('error loading libfuse.so.2'). Th
- **App identity: the host.anselm.anselm bundle ID is doubly broken AND reverse-DNS resolves to the BANNED anselm.host domain** — The app-identity section already flags this, but it is a blocking prerequisite for EVERY signing step (codesign, AUMID, MSIX identity, anselm:// registration, notification keying, data-dir path) and t
- **Legal: runtime-DOWNLOADED language runtimes (python/node/uv/dotnet) attribution + EULA execution-of-arbitrary-code + AI-output disclaimers** — Because Anselm downloads-and-execs python/node/uv/dotnet at runtime (not bundled), THIRD_PARTY_NOTICES must explicitly state these are user-fetched, not redistributed (different legal posture — you av
- **CI: macOS signing+notarization secrets in GitHub Actions (cert .p12 + App Store Connect API key .p8) + keychain setup on ephemeral runners** — The build-release-ci/fastforge section needs the concrete secret inventory and runner ritual: Developer ID Application cert exported as base64 .p12 + password in secrets; App Store Connect API key (.p
- **Timeline/accounts: Apple individual enrollment 24-48h vs org D-U-N-S 7-10 days — buy-order gate** — Concrete procurement timing to encode: Apple Developer Program $99/yr, INDIVIDUAL approves in 1-3 days, ORG needs a D-U-N-S (2-3 days to verify) + 7-10 days total. This directly gates the macOS critic
- **GitHub Releases as appcast host: raw-asset URL stability + the actual ban-resistance failure mode** — The ban-resistance thesis rests on GitHub Releases raw asset URLs, but two operational facts must be recorded: (1) GitHub release asset download URLs are stable per-tag (github.com/<o>/<r>/releases/do