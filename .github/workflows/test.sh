#!/bin/sh
set -e
cue export --out yaml ci.cue > ci.yml
git add .
git commit --allow-empty -m 'testing ci'
git push
