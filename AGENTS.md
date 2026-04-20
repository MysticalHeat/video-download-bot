# AGENTS

## Deploy

- Production host: `NomliHost`
- Production service: `cobalt-telegram-bot`
- Production binary path: `/opt/cobalt-telegram-bot/cobalt-telegram-bot`
- Production env file: `/etc/cobalt-telegram-bot.env`
- Systemd unit path: `/etc/systemd/system/cobalt-telegram-bot.service`
- `HomeVpn` is not used for this service unless explicitly reconfigured.

## Standard deploy flow

From the repo root:

```bash
make deploy
```

Equivalent direct script usage:

```bash
./scripts/deploy.sh
```

Equivalent manual flow:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/cobalt-telegram-bot ./cmd/cobalt-telegram-bot
rsync -av --progress dist/cobalt-telegram-bot NomliHost:/tmp/cobalt-telegram-bot.new
ssh NomliHost "install -m 755 /tmp/cobalt-telegram-bot.new /opt/cobalt-telegram-bot/cobalt-telegram-bot && rm -f /tmp/cobalt-telegram-bot.new && systemctl restart cobalt-telegram-bot && systemctl status cobalt-telegram-bot --no-pager"
```

## Verification

```bash
ssh NomliHost "systemctl is-active cobalt-telegram-bot"
ssh NomliHost "journalctl -u cobalt-telegram-bot -n 50 --no-pager"
```

## Notes

- The service currently runs via `systemd`, not Docker.
- If deploy changes the binary only, restarting `cobalt-telegram-bot` is enough.
- Do not assume `HomeVpn` has the service installed.
- Direct `scp` into `/opt/cobalt-telegram-bot/` may fail; use `/tmp` + `install`.
- Prefer `rsync` over `scp` for the upload step; it was more reliable here.
- Script entrypoint: `./scripts/deploy.sh`.
- Make target: `make deploy`.
