#!/system/bin/sh
# Answer one HTTP request on the socket wired to stdin/stdout.
#
# Run by `nc -L`, which passes no arguments, so the name comes in through the
# NAME environment variable.
#
# CRLF is emitted by printf at the point of use rather than stored in a
# variable: command substitution strips trailing newlines, so
# CRLF=$(printf '\r\n') would yield a bare CR and clients would reject the
# response.
#
# The separator comes from SP because a literal space followed by digits is
# lost on the way into this file.
SP=$(printf "\x20")
BODY="${NAME:-nomad}"
printf "HTTP/1.1%s200%sOK\r\n" "$SP" "$SP"
printf "Content-Type:%stext/plain\r\n" "$SP"
printf "Content-Length:%s%d\r\n" "$SP" "${#BODY}"
printf "Connection:%sclose\r\n\r\n" "$SP"
printf "%s" "$BODY"
