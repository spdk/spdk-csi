#!/bin/bash -e

helmPath=charts/spdk-csi
LOG=/tmp/yamllint.log

yamllint -s -c scripts/yamllint.yml -f parsable $helmPath/*.yaml
yamllint -s -c scripts/yamllint.yml -f parsable $helmPath/templates/*.yaml | grep -v -e "line too long" -e "too many spaces inside braces" -e "missing document start" -e "syntax error" > $LOG

linecount=$(wc -l < $LOG)
if [ "$linecount" -gt 0 ]; then
	cat $LOG
	exit 1
fi
