#!/usr/bin/env bash

set -eu

if [[ "${TRAVIS}" != "true" ]]; then
  echo "This script is allowed to run on Travis CI"
  exit 1
fi

REPO=taildog

git config --global user.email "shiketaudonko41@gmail.com"
git config --global user.name "kamatama41"
git remote -v
git remote add kamatama41 https://${GITHUB_USER}:${GITHUB_TOKEN}@github.com/kamatama41/${REPO}.git
git fetch kamatama41
git checkout -b master kamatama41/master

PROJECT_ROOT=$(cd $(dirname $0)/..; pwd)
VERSION_FILE=${PROJECT_ROOT}/version.go
VERSION=$(cat ${VERSION_FILE} | grep -o -E "[0-9]+\.[0-9]+\.[0-9]+")

echo "## Create the new release"
go get github.com/aktau/github-release
github-release release \
  --user kamatama41 \
  --repo ${REPO} \
  --tag v${VERSION}


echo "## Build and upload release binaries"
PLATFORMS="darwin/amd64"
PLATFORMS="${PLATFORMS} windows/amd64"
PLATFORMS="${PLATFORMS} linux/amd64"
for PLATFORM in ${PLATFORMS}; do
  GOOS=${PLATFORM%/*}
  GOARCH=${PLATFORM#*/}
  BIN_FILENAME="${REPO}"
  CMD="GOOS=${GOOS} GOARCH=${GOARCH} go build -o ${BIN_FILENAME}"
  rm -f ${BIN_FILENAME}
  echo "${CMD}"
  eval ${CMD}

  ZIP_FILENAME="${REPO}_v${VERSION}_${GOOS}_${GOARCH}.zip"
  CMD="zip ${ZIP_FILENAME} ${BIN_FILENAME}"
  echo "${CMD}"
  eval ${CMD}

  github-release upload \
    --user kamatama41 \
    --repo ${REPO} \
    --tag v${VERSION} \
    --name ${ZIP_FILENAME} \
    --file ${ZIP_FILENAME}
done

gem install semantic
script=$(cat << EOS
require 'semantic'
puts Semantic::Version.new(gets).increment!(:patch)
EOS
)
NEXT_VERSION=$(echo ${VERSION} | ruby -e "${script}")
cat << EOS > ${VERSION_FILE}
package main

var VERSION = "${NEXT_VERSION}"
EOS


echo "## Bump up the version to ${NEXT_VERSION}"
git add ${VERSION_FILE}
git commit -m "Bump up to the next version"
git push kamatama41 master
