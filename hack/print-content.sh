#!/bin/bash

cat docs/proposals/multi-node/README.md | gh-md-toc - | sed 's/\*/-/g' | sed 's/   /  /g' | sed 's/^  //'
