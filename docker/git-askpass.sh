#!/bin/sh
# git-askpass.sh - Non-interactive credential helper for git in headless containers
case "$1" in
    *Username*|*username*)
        echo "${GIT_USERNAME:-x-access-token}"
        ;;
    *)
        echo "${GITHUB_TOKEN:-${GIT_TOKEN:-$GIT_PASSWORD}}"
        ;;
esac
