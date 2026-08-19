#!/bin/sh

echo
echo "Job started: $(date)"
# shellcheck disable=SC2086 # SYNC_CMD is intentionally unquoted so subcommand arguments split, e.g. "prs refresh"
ghp-sync $SYNC_CMD
echo "Job finished: $(date)"
