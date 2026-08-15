#!/bin/sh
set -eu
printf '%s mock component\n' "$1"
exec sleep infinity
