#!/bin/bash
set -euo pipefail

build_binary() {
  rm -rf ./dist
  mkdir -p ./dist
  bazel build --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 //cmd/protoc-gen-doc
  if [[ "$OSTYPE" == "darwin"* ]]; then
    cp bazel-bin/cmd/protoc-gen-doc/linux_amd64_pure_stripped/protoc-gen-doc dist/protoc-gen-doc
  else
    cp bazel-bin/src/go/cmd/protoc-gen-doc/linux_amd64_stripped/protoc-gen-doc dist/protoc-gen-doc
  fi
}

build_and_tag_image() {
  docker build -t "${1}" .
  docker tag "${1}" "${2}:${3}"
  docker tag "${1}" "${2}:latest"
}

push_image() {
  # credentials are encrypted in travis.yml
  docker login -u "${DOCKER_HUB_USER}" -p "${DOCKER_HUB_PASSWORD}"
  docker push "${1}"
  docker push "${2}:${3}"
  docker push "${2}:latest"
}

main() {
  local sha="${TRAVIS_COMMIT:-}"
  if [ -z "${sha}" ]; then sha=$(git rev-parse HEAD); fi

  local repo="pseudomuto/protoc-gen-doc"
  local version="$(grep "const VERSION" "version.go" | awk '{print $NF }' | tr -d '"')"
  local git_tag="${repo}:${sha}"

  build_binary
  build_and_tag_image "${git_tag}" "${repo}" "${version}"

  if [ -n "${DOCKER_HUB_USER:-}" ]; then
    push_image "${git_tag}" "${repo}" "${version}"
  fi
}

main "$@"
