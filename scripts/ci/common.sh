#!/bin/bash -e

function export_proxy() {
    local http_proxies

    http_proxies=$(env | { grep -Pi "http[s]?_proxy" || true; })
    [ -z "$http_proxies" ] && return 0

    for proxy in $http_proxies; do
        # shellcheck disable=SC2001,SC2005
        echo "$(sed "s/.*=/\U&/" <<< "$proxy")"
        # shellcheck disable=SC2001
        export "$(sed "s/.*=/\U&/" <<< "$proxy")"
    done

    export NO_PROXY="$NO_PROXY,127.0.0.1,localhost,10.0.0.0/8,192.168.0.0/16,.internal"
    export no_proxy="$no_proxy,127.0.0.1,localhost,10.0.0.0/8,192.168.0.0/16,.internal"
}
