# Runtime guard shared by the systemd service and LaunchAgent.
token_file=$1
state_token=$2
shift 2

if [ ! -f "$token_file" ] || [ ! -r "$token_file" ] || [ ! -s "$token_file" ] \
  || ! LC_ALL=C grep -q '[^[:space:]]' "$token_file"; then
  echo 'Surge: configured tokenFile must exist and contain a readable, non-empty token.' >&2
  exit 1
fi

# Validate the path Surge actually reads too: a missing or replaced Home Manager
# link must not let Surge fall back to a generated token.
if [ ! "$token_file" -ef "$state_token" ]; then
  echo 'Surge: state token must point to the configured tokenFile.' >&2
  exit 1
fi

# Keep the validated token file authoritative over the inherited environment.
unset SURGE_TOKEN
exec "$@"
