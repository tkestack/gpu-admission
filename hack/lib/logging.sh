#!/bin/bash

api::log::usage() {
  echo >&2
  local message
  for message; do
    echo "$message" >&2
  done
  echo >&2
}

 # Print a status line.  Formatted to show up in a stream of output.
api::log::status() {
  timestamp=$(date +"[%m%d %H:%M:%S]")
  echo "+++ $timestamp $1"
  shift
  for message; do
    echo "    $message"
  done
}
# vim: set ts=2 sw=2 tw=0 et :
