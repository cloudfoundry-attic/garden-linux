#!/bin/bash

[ -n "$DEBUG" ] && set -o xtrace
set -o nounset
set -o errexit
shopt -s nullglob

filter_forward_chain="${GARDEN_IPTABLES_FILTER_FORWARD_CHAIN}"
filter_default_chain="${GARDEN_IPTABLES_FILTER_DEFAULT_CHAIN}"
filter_instance_prefix="${GARDEN_IPTABLES_FILTER_INSTANCE_PREFIX}"
nat_prerouting_chain="${GARDEN_IPTABLES_NAT_PREROUTING_CHAIN}"
nat_postrouting_chain="${GARDEN_IPTABLES_NAT_POSTROUTING_CHAIN}"
nat_instance_prefix="${GARDEN_IPTABLES_NAT_INSTANCE_PREFIX}"
interface_name_prefix="${GARDEN_NETWORK_INTERFACE_PREFIX}"

function teardown_deprecated_rules() {
  # Remove jump to garden-dispatch from INPUT
  iptables -w -S INPUT 2> /dev/null |
    grep " -j garden-dispatch" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Remove jump to garden-dispatch from FORWARD
  iptables -w -S FORWARD 2> /dev/null |
    grep " -j garden-dispatch" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Prune garden-dispatch
  iptables -w -F garden-dispatch 2> /dev/null || true

  # Delete garden-dispatch
  iptables -w -X garden-dispatch 2> /dev/null || true
}

function teardown_filter() {
  teardown_deprecated_rules

  # Prune garden-forward chain
  iptables -w -S ${filter_forward_chain} 2> /dev/null |
    grep "\-g ${filter_instance_prefix}" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Prune per-instance chains
  iptables -w -S 2> /dev/null |
    grep "^-A ${filter_instance_prefix}" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Delete per-instance chains
  iptables -w -S 2> /dev/null |
    grep "^-N ${filter_instance_prefix}" |
    sed -e "s/-N/-X/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  # Remove jump to garden-forward from FORWARD
  iptables -w -S FORWARD 2> /dev/null |
    grep " -j ${filter_forward_chain}" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w

  iptables -w -F ${filter_forward_chain} 2> /dev/null || true
  iptables -w -F ${filter_default_chain} 2> /dev/null || true
}

function setup_filter() {
  teardown_filter

  # Create or flush forward chain
  iptables -w -N ${filter_forward_chain} 2> /dev/null || iptables -w -F ${filter_forward_chain}
  iptables -w -A ${filter_forward_chain} -j DROP

  # Create or flush default chain
  iptables -w -N ${filter_default_chain} 2> /dev/null || iptables -w -F ${filter_default_chain}

  # Always allow established connections to containers
  iptables -w -A ${filter_default_chain} -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

  # Forward outbound traffic via ${filter_forward_chain}
  iptables -w -A FORWARD -i ${GARDEN_NETWORK_INTERFACE_PREFIX}+ --jump ${filter_forward_chain}

  # Forward inbound traffic immediately
  default_interface=$(ip route show | grep default | cut -d' ' -f5 | head -1)
  iptables -w -I ${filter_forward_chain} -i $default_interface --jump ACCEPT
}

function teardown_nat() {
  # Prune prerouting chain
  iptables -w -t nat -S ${nat_prerouting_chain} 2> /dev/null |
    grep "\-j ${nat_instance_prefix}" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w -t nat

  # Prune per-instance chains
  iptables -w -t nat -S 2> /dev/null |
    grep "^-A ${nat_instance_prefix}" |
    sed -e "s/-A/-D/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w -t nat

  # Delete per-instance chains
  iptables -w -t nat -S 2> /dev/null |
    grep "^-N ${nat_instance_prefix}" |
    sed -e "s/-N/-X/" -e "s/\s\+\$//" |
    xargs --no-run-if-empty --max-lines=1 iptables -w -t nat

  # Flush prerouting chain
  iptables -w -t nat -F ${nat_prerouting_chain} 2> /dev/null || true

  # Flush postrouting chain
  iptables -w -t nat -F ${nat_postrouting_chain} 2> /dev/null || true
}

function setup_nat() {
  teardown_nat

  # Create prerouting chain
  iptables -w -t nat -N ${nat_prerouting_chain} 2> /dev/null || true

  # Bind chain to PREROUTING
  (iptables -w -t nat -S PREROUTING | grep -q "\-j ${nat_prerouting_chain}\b") ||
    iptables -w -t nat -A PREROUTING \
      --jump ${nat_prerouting_chain}

  # Bind chain to OUTPUT (for traffic originating from same host)
  (iptables -w -t nat -S OUTPUT | grep -q "\-j ${nat_prerouting_chain}\b") ||
    iptables -w -t nat -A OUTPUT \
      --out-interface "lo" \
      --jump ${nat_prerouting_chain}

  # Create postrouting chain
  iptables -w -t nat -N ${nat_postrouting_chain} 2> /dev/null || true

  # Bind chain to POSTROUTING
  (iptables -w -t nat -S POSTROUTING | grep -q "\-j ${nat_postrouting_chain}\b") ||
    iptables -w -t nat -A POSTROUTING \
      --jump ${nat_postrouting_chain}
}

case "${1}" in
  setup)
    setup_filter
    setup_nat

    # Enable forwarding
    echo 1 > /proc/sys/net/ipv4/ip_forward
    ;;
  teardown)
    teardown_filter
    teardown_nat
    ;;
  *)
    echo "Unknown command: ${1}" 1>&2
    exit 1
    ;;
esac
