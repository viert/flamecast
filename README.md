# FlameCast

FlameCast is a proof of concept analog of the famous Icecast server written in Go language.

### Building

```
go get github.com/op/go-logging
go get github.com/viert/properties
go get github.com/viert/endless
go build src/flamecast.go
```

### Running
```
flamecast -c config_file
```

### Example config file

```
[main]

# Bind host:port in Go "net" package format
# Example:
#    bind = localhost:8000
#    bind = 0.0.0.0:7500
#    bind = :3000

bind = :8000

# Logging properties

log.file = flamecast.log
log.level = debug

[sources.shuffle]
# These are icecast-compatible source tags. Valid until overwritten by a relay
# or a source feeder client. 
#

source.name =
source.description = 
source.genre =
source.public =
source.site =
source.bitrate =

# Source type configuration. Valid types are "push" and "pull"
# PUSH sources wait for libshout compatible feeder while PULL
# sources get stream via http (this may be used as icecast's
# relay feature)

source.type = push

# Source fallback is the name of source that will be streamed to
# clients in case the main source is not available. When the main
# source restores, clients are automatically moved to it back from
# fallback source

source.fallback = viertfm

# Source auth.user and auth.password are libshout-compatible
# credentials for PUSH sources (basically it's HTTP basic auth)
# PUSH sources may not be unprotected, i.e. not having configured
# user/password you just get your source unaccessible

source.auth.user = source
source.auth.password = passw0rd

# Broadcast auth.type is the type of auth for source listeners.
# Valid types are "token" and "none". In "token" mode flamecast
# waits for ?token= parameter from listeners and then forward it
# to the URL configured in broadcast.auth.token_check_url.
# If the response from token_check_url contains one of the following
# headers
#   icecast-auth-user: 1
#   flamecast-auth-user: 1
# it validates user and begins streaming, otherwise 401 Forbidden
# is sent to listener
# 
# If "token" parameter is absent in query flamecast will look for token
# in X-Flamecast-Token header or for string "Token <token>" in Authorization
# header. If there's no token found flamecast considers user as unauthorized

#broadcast.auth.type = token
#broadcast.auth.token_check_url = http://localhost/auth/token

# broadcast.notify.enter and broadcast.notify.leave are the notify
# handlers. Flamecast will send listener properties via POST to this
# urls.

broadcast.notify.enter = http://localhost/auth/enter
broadcast.notify.leave = http://localhost/auth/leave

[sources.viertfm]
source.type = pull

# source.url for PULL sources is the URL flamecast requests to get the
# data for the source
source.url = http://viert.fm/stream/shuffle128
```
