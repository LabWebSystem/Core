#!/bin/sh
set -eu
printf '%sモックコンポーネント\n' "$1"
exec sleep infinity
