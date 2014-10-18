#!/bin/bash
set -e

# Things to look out for:
# 1) We generate into a *.tmp file first, otherwise go run will pick up the
#    initially empty *.go file, and fail.
# 2) We re-write occurrences of $VEYRON_ROOT to the literal \$VEYRON_ROOT, to
#    keep flag documentation generic.

{
  echo '// This file was auto-generated via go generate.'
  echo '// DO NOT UPDATE MANUALLY'
  echo ''
  echo '/*'
  veyron go run *.go help -style=godoc ... | sed s:$VEYRON_ROOT:\$VEYRON_ROOT:g
  echo '*/'
  echo 'package main'
} > ./doc.go.tmp
mv ./doc.go.tmp ./doc.go
