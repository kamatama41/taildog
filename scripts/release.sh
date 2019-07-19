#!/usr/bin/env bash

set -eu

if [[ "${TRAVIS}" != "true" ]]; then
  echo "This script is allowed to run on Travis CI"
  exit 1
fi

REPO=taildog

git config --local user.email "shiketaudonko41@gmail.com"
git config --local user.name "${GITHUB_USER}"
git remote -v
git remote add kamatama41 https://${GITHUB_USER}:${GITHUB_TOKEN}@github.com/kamatama41/${REPO}.git
git fetch kamatama41
git checkout -b master kamatama41/master

PROJECT_ROOT=$(cd $(dirname $0)/..; pwd)
VERSION_FILE=${PROJECT_ROOT}/version
VERSION=$(cat ${VERSION_FILE})

git tag -a ${VERSION} -m " release"
git push origin ${VERSION}

curl -sL https://git.io/goreleaser | bash

script=$(cat << EOS
require 'semantic'
puts Semantic::Version.new(gets[1..-1]).increment!(:patch)
EOS
)
NEXT_VERSION=v$(echo ${VERSION} | ruby -e "${script}")
cat << EOS > ${VERSION_FILE}
${NEXT_VERSION}
EOS


echo "## Bump up the version to ${NEXT_VERSION}"
git add ${VERSION_FILE}
git commit -m "Bump up to the next version"
git push kamatama41 master
