#!/usr/bin/env bash
set -euo pipefail

HOST="${DEPLOY_HOST:-NomliHost}"
SERVICE="${DEPLOY_SERVICE:-cobalt-telegram-bot}"
BINARY_NAME="cobalt-telegram-bot"
REMOTE_DIR="${DEPLOY_REMOTE_DIR:-/opt/cobalt-telegram-bot}"
REMOTE_BIN="${REMOTE_DIR}/${BINARY_NAME}"
REMOTE_TMP="/tmp/${BINARY_NAME}.new"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"
LOCAL_BIN="${DIST_DIR}/${BINARY_NAME}"

cd "${ROOT_DIR}"

echo ">>> Building ${BINARY_NAME}"
mkdir -p "${DIST_DIR}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "${LOCAL_BIN}" ./cmd/${BINARY_NAME}

echo ">>> Uploading to ${HOST}:${REMOTE_TMP}"
rsync -av --progress "${LOCAL_BIN}" "${HOST}:${REMOTE_TMP}"

LOCAL_SIZE="$(stat -c '%s' "${LOCAL_BIN}")"
REMOTE_SIZE="$(ssh "${HOST}" "stat -c '%s' '${REMOTE_TMP}'")"

if [[ "${LOCAL_SIZE}" != "${REMOTE_SIZE}" ]]; then
	echo "Upload verification failed: local=${LOCAL_SIZE} remote=${REMOTE_SIZE}" >&2
	ssh "${HOST}" "rm -f '${REMOTE_TMP}'"
	exit 1
fi

echo ">>> Installing binary and restarting ${SERVICE}"
ssh "${HOST}" "install -m 755 '${REMOTE_TMP}' '${REMOTE_BIN}' && rm -f '${REMOTE_TMP}' && systemctl restart '${SERVICE}' && systemctl status '${SERVICE}' --no-pager -n 20"

echo ">>> Verifying ${SERVICE}"
ssh "${HOST}" "systemctl is-active '${SERVICE}' && journalctl -u '${SERVICE}' -n 20 --no-pager"

echo ">>> Deploy complete"
