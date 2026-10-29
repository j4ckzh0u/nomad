#!/system/bin/sh
# Minimal HTTP service for Nomad placement tests.
#
# Port and name are baked in because passing them on a command line
# risks a space and the following digits being joined together by the
# tooling that writes these files.
#
# nc -L runs the responder per connection with the socket on stdin and
# stdout. NAME is exported because nc passes no arguments through.

PORT=9802
NAME=svc-b
export NAME

exec toybox nc -s0.0.0.0 -p "$PORT" -L /data/local/tmp/respond.sh
