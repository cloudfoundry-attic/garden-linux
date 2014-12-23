#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

cd $(dirname "${0}")

source ./etc/config

filter_forward_chain="${GARDEN_IPTABLES_FILTER_FORWARD_CHAIN}"
filter_default_chain="${GARDEN_IPTABLES_FILTER_DEFAULT_CHAIN}"
filter_instance_prefix="${GARDEN_IPTABLES_FILTER_INSTANCE_PREFIX}"
nat_prerouting_chain="${GARDEN_IPTABLES_NAT_PREROUTING_CHAIN}"
nat_postrouting_chain="${GARDEN_IPTABLES_NAT_POSTROUTING_CHAIN}"
nat_instance_prefix="${GARDEN_IPTABLES_NAT_INSTANCE_PREFIX}"
interface_name_prefix="${GARDEN_NETWORK_INTERFACE_PREFIX}"

filter_instance_chain="${filter_instance_prefix}${id}"
nat_instance_chain="${filter_instance_prefix}${id}"

function teardown_filter() {
  # Prune forward chain
  iptables -w -S ${filter_forward_chain} 2> /dev/null |
    grep "\-g ${filter_instance_chain}\b" |
    sed -e "s/-A/-D/" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Flush and delete instance chain
  iptables -w -F ${filter_instance_chain} 2> /dev/null || true
  iptables -w -X ${filter_instance_chain} 2> /dev/null || true
}

function setup_filter() {
  teardown_filter

  # Create instance chain
  iptables -w -N ${filter_instance_chain}

  # Allow intra-subnet traffic (Linux ethernet bridging goes through ip stack)
  iptables -w -A ${filter_instance_chain} -s ${network_cidr} -d ${network_cidr} -j ACCEPT

  iptables -w -A ${filter_instance_chain} \
    --goto ${filter_default_chain}

  # Bind instance chain to forward chain
  iptables -w -I ${filter_forward_chain} 2 \
    --in-interface ${bridge_iface} \
    --goto ${filter_instance_chain}
}

function teardown_nat() {
  # Prune prerouting chain
  iptables -w -t nat -S ${nat_prerouting_chain} 2> /dev/null |
    grep "\-j ${nat_instance_chain}\b" |
    sed -e "s/-A/-D/" |
    xargs --no-run-if-empty --max-lines=1 iptables -w -t nat

  # Flush and delete instance chain
  iptables -w -t nat -F ${nat_instance_chain} 2> /dev/null || true
  iptables -w -t nat -X ${nat_instance_chain} 2> /dev/null || true
}

function setup_nat() {
  teardown_nat

  # Create instance chain
  iptables -w -t nat -N ${nat_instance_chain}

  # Bind instance chain to prerouting chain
  iptables -w -t nat -A ${nat_prerouting_chain} \
    --jump ${nat_instance_chain}

  # Enable NAT for traffic coming from containers
  (iptables -w -t nat -S ${nat_postrouting_chain} | grep "\-j SNAT\b" | grep -q -F -- "-s ${network_cidr}") ||
    iptables -w -t nat -A ${nat_postrouting_chain} \
      --source ${network_cidr} \
      --jump SNAT \
      --to $external_ip
}

case "${1}" in
  "setup")
    setup_filter
    setup_nat

    ;;

  "teardown")
    teardown_filter
    teardown_nat

    ;;

  "in")
    if [ -z "${HOST_PORT:-}" ]; then
      echo "Please specify HOST_PORT..." 1>&2
      exit 1
    fi

    if [ -z "${CONTAINER_PORT:-}" ]; then
      echo "Please specify CONTAINER_PORT..." 1>&2
      exit 1
    fi

    iptables -w -t nat -A ${nat_instance_chain} \
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
