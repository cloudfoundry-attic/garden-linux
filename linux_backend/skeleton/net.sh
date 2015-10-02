#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname "${0}")

source ./etc/config

filter_instance_prefix="${GARDEN_IPTABLES_FILTER_INSTANCE_PREFIX}"
nat_instance_chain="${filter_instance_prefix}${id}"

case "${1}" in
   "in")
    if [ -z "${HOST_PORT:-}" ]; then
      echo "Please specify HOST_PORT..." 1>&2
      exit 1
    fi

    if [ -z "${CONTAINER_PORT:-}" ]; then
      echo "Please specify CONTAINER_PORT..." 1>&2
      exit 1
    fi

    iptables --wait --table nat -A ${nat_instance_chain} \
      --protocol tcp \
      --destination "${external_ip}" \
      --destination-port "${HOST_PORT}" \
      --jump DNAT \
      --to-destination "${network_container_ip}:${CONTAINER_PORT}"

    ;;

  "get_ingress_info")
    if [ -z "${ID:-}" ]; then
      echo "Please specify container ID..." 1>&2
      exit 1
    fi
    tc filter show dev ${network_host_iface} parent ffff:

    ;;
  "get_egress_info")
    if [ -z "${ID:-}" ]; then
      echo "Please specify container ID..." 1>&2
      exit 1
    fi
    tc qdisc show dev ${network_host_iface}

    ;;
  *)
    echo "Unknown command: ${1}" 1>&2
    exit 1

    ;;
esac
