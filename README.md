# lolticker-agent

A headless companion app for the LoL live ticker. It reads the League of
Legends live client API on your own machine and streams a stripped-down
version of it to one server you configure.

This is the only piece of software you install, so the whole thing is meant to
be readable in about fifteen minutes. Start with
[`internal/wire/types.go`](internal/wire/types.go) — that file is the complete
list of everything that can leave your machine.

## What it reads

Exactly one endpoint, on loopback only:

- `https://127.0.0.1:2999/liveclientdata/allgamedata`

That is the entire list. See [`internal/riot/client.go`](internal/riot/client.go).

## What it sends

To the one server in your config file, over a single WebSocket:

- `snapshot` — game mode, map, clock, and per player: Riot ID, champion, skin,
  team, position, level, K/D/A, CS, ward score, item IDs, summoner spells, and
  the keystone rune plus both rune tree names.
- `events` — kills, turrets, dragons, barons and so on, as the game reports
  them, plus role-quest completions the agent infers (marked `syn`, see below).
- `gameEnd` — win/lose when the game is over.
- `idle` — a heartbeat every 30 s while you are not in a game.

Optionally (`shareActivePlayer`, on by default) a `snapshot` also carries your
own gold, current/max HP, champion stats and ability ranks. Turn it off with
`--no-active` or `"shareActivePlayer": false`.

## What it does not send

No file contents, no process list, no hardware info, no account credentials, no
chat, no telemetry or crash reporting, no analytics of any kind. Item and spell
descriptions, and your rune pages, are dropped in the transform step and never
leave the process. There is no second network destination: the only outbound
connection is the WebSocket to your configured server.

## Inferred events

The 2026 role quests fire no event of their own, so the agent infers three of
them from state it already polls — an upgraded Teleport or a level above 18
(top), tier-3 boots (mid), an eighth inventory slot (bot) — and sends them as
normal events flagged `"syn": true`. Nothing extra is read to do this; it is
the same once-a-second payload, interpreted. Jungle and support get no event,
because 26.01 gave them tuning rather than an observable reward.

Rune data comes out of that same payload too. The agent never calls
`/playermainrunes`.

## Why `InsecureSkipVerify` is in there

The game client serves a self-signed certificate issued for `*.riotgames.com`
on a loopback address, so hostname verification can never pass no matter what
you do. The setting is scoped to the one HTTP client that talks to
`127.0.0.1:2999` and is never used for any other host — the relay connection
uses normal TLS verification. A later version will pin Riot's root certificate
with an embedded copy instead.

## Install and configure

Config lives at:

- Windows: `%AppData%\lolticker\config.json`
- Linux/macOS: `~/.config/lolticker/config.json`

```json
{
  "server": "wss://lol.erxt.dev/ws/agent",
  "token": "lt_xxxxxxxxxxxxxxxxxxxx",
  "shareActivePlayer": true
}
```

Get the token by signing in on the ticker site. Running the agent with nothing
configured prints these instructions and exits non-zero — it never prompts.

## Usage

```
lolticker-agent [flags]

  --config PATH     alternate config file
  --server URL      relay WebSocket URL
  --token TOKEN     agent token (env LOLTICKER_TOKEN also accepted)
  --no-active       do not send gold/stats/abilities of the local player
  --dump PATH       record raw allgamedata to a JSONL file instead of sending
  --once            single poll, print lean snapshot to stdout, exit
  --verbose         debug logging
  --version
```

`--once` is the quickest way to see exactly what would be sent:

```bash
lolticker-agent --once
```

`--dump game.jsonl` records the raw payload once a second and sends nothing
anywhere; attach that file to a bug report.

Leave the agent running — it polls every 5 s while you are out of a game and
once a second while you are in one, and returns to idle when the game ends.
Ctrl-C closes the connection cleanly.

## The desktop app

Running the agent with no flags opens a window: status, your share link, the
token field, the settings toggles and a log tail. Closing the window hides it
to the tray and the agent keeps streaming — **Quit in the tray menu is the only
thing that actually stops it.** The tray icon tells you the state at a glance:

| Icon | Meaning |
|---|---|
| Ring | Offline — not connected to the relay |
| Solid dot | Online — connected, no game |
| Dot with a notch | In a game, streaming |
| Triangle | Something is wrong; open the window to read it |

They differ by shape rather than colour, because a tray renders them at 16 px
against a background nobody controls.

Only one agent runs at a time. Launching a second one raises the first one's
window and exits — two agents would fight over the relay connection forever,
since the relay drops the older connection whenever a new one authenticates
with the same token.

### Headless

`go build -tags nogui` produces a pure-Go, cgo-free binary with no display
dependency and no Fyne in it at all. Use it on a headless box, or when you want
the smallest thing to audit. `--once` and `--dump` behave identically in both
builds.

## Things Windows will do to you

**SmartScreen.** The release binaries are not code-signed — an Authenticode
certificate costs real money every year and this is a hobby project. Windows
will therefore warn you when you download and first run it. Click "More info"
then "Run anyway", or verify the download yourself first: every release ships a
`SHA256SUMS` file beside the binaries.

**Antivirus false positives.** A Go binary that does network I/O and rewrites
itself is a common heuristic false positive. There is no code fix; if it
happens, it gets submitted to Microsoft's false-positive form.

**A `.old` file.** After an update you will see `lolticker-agent.exe.old` next
to the binary. Windows will not let a running executable be deleted, so the
updater renames the previous version instead. It is harmless and you can delete
it once the new version has started.

## Updates

The agent checks for a new release every six hours and, if it finds one, shows
a quiet line in the window and a menu item in the tray. **Nothing downloads or
installs until you click it**, and nothing is ever applied while you are in a
game — if you click during a match it waits until the game ends.

Every release is signed with an ed25519 key whose public half is compiled into
the binary, so an update that is not signed by the real key cannot be applied.

## Build from source

Requires Go 1.22+. The desktop build needs cgo and OpenGL, because Fyne does;
the headless build needs neither.

```bash
go test ./...

# desktop (needs a C compiler; -H=windowsgui stops a console window appearing)
go build -ldflags "-H=windowsgui" -o lolticker-agent.exe ./cmd/lolticker-agent

# headless, pure Go
CGO_ENABLED=0 go build -tags nogui -o lolticker-agent ./cmd/lolticker-agent
```

On Linux the desktop build also needs `libgl1-mesa-dev` and `xorg-dev`. There
is no cross-compiling the desktop build: it is built natively per platform in
CI.

To verify a published release, build it yourself with the tag checked out and
compare against the published SHA-256:

```bash
sha256sum lolticker-agent
```

## Disclaimer

lolticker isn't endorsed by Riot Games and doesn't reflect the views or
opinions of Riot Games or anyone officially involved in producing or managing
Riot Games properties.
