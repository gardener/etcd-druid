#!/usr/bin/env bash

set -e

echo "> Adding Apache License header to all go files where it is not present"

# Uses the tool https://github.com/google/addlicense
YEAR=$1
addlicense -v -c "SAP SE or an SAP affiliate company" -y "${YEAR}" -l apache -ignore "vendor/**" -ignore "**/*.md" -ignore "**/*.yaml" -ignore "**/Dockerfile" .
