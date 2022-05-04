#!/usr/bin/env bash

# Mount local binaries to enable HOTRELOAD after a deployment.
#
# It helps to run recently build binaries (i.e. from `make fast-central`) inside
# the cluster by only deleting the pod, instead of building a new main image.

set -eo pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source "${DIR}"/../deploy/common/k8sbased.sh

usage() {
    cat >&2 <<EOF
Usage: $(basename $0) [-h] [-u] [-n <node-type>] {sensor,central,migrator,admission}

Options:

    -n <node-type>
        Select a node type different from local cluster. The binary will be sent
        send the binary to the POD's node using \`scp\`, supported
        <node-types>s: ec2, minikube

    -u
        Don't patch the POD, only upload the binary.

    -h
        Print this help and exit.
EOF
    exit 1
}

node_type=
upload_only=

while getopts ":n:uh" o; do
    case "${o}" in
        n)
            node_type=${OPTARG}
            echo "${node_type}" | grep -qE '(ec2)|(minikube)' || usage
            ;;
        u)
            upload_only=1
            ;;
        h)
            usage
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND-1))

if [[ -z "$1" ]]; then
  echo "Expected component as the first arg"
  echo "Available [sensor, central, migrator, admission]"
  exit 1
fi
set -x
component="$1"

case "${component}" in
"sensor")
  hotload_binary bin/kubernetes-sensor kubernetes sensor ${node_type} ${upload_only}
  ;;
"central")
  hotload_binary central central central ${node_type} ${upload_only}
  ;;
"migrator")
  hotload_binary bin/migrator migrator central ${node_type} ${upload_only}
  ;;
"admission"|"admission-control"|"admission-controller")
  hotload_binary admission-control admission-control admission-control ${node_type} ${upload_only}
  ;;
*)
  echo "Invalid input: ${component}"
  usage
  ;;
esac
