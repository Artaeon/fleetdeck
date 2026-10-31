#!/usr/bin/env python3
"""Render scoped, temporary rules for an operator-managed proxy handover.

This module does not execute commands or manage containers. The operator must
own the named table, validate the candidate, and arm a phase-aware rollback.
"""
import ipaddress
import re

TABLE = "fleetdeck_proxy_handover"


def validate(interface, host4, host6, target4, http6, https6):
    if not isinstance(interface, str) or not re.fullmatch(r"[a-zA-Z0-9_.-]{1,15}", interface):
        raise ValueError("invalid interface")
    addresses = [ipaddress.ip_address(x) for x in (host4, host6, target4)]
    if [a.version for a in addresses] != [4, 6, 4]:
        raise ValueError("expected IPv4, IPv6 and IPv4 addresses")
    if any(a.is_unspecified or a.is_multicast or a.is_loopback or a.is_link_local for a in addresses):
        raise ValueError("expected unicast non-loopback addresses")
    if not addresses[2].is_private:
        raise ValueError("candidate must be on a private container network")
    if any(type(p) is not int or not 1024 <= p <= 65535 for p in (http6, https6)) or http6 == https6:
        raise ValueError("distinct unprivileged alternate ports required")
    return interface, *(str(a) for a in addresses), http6, https6


def render(interface, host4, host6, target4, http6, https6):
    interface, host4, host6, target4, http6, https6 = validate(interface, host4, host6, target4, http6, https6)
    # One nft batch replaces only our table. Docker/UFW rules remain untouched.
    lines = [f"add table inet {TABLE}", f"flush table inet {TABLE}"]
    for chain, hook in (("incoming", "prerouting"), ("local", "output")):
        lines.append(f"add chain inet {TABLE} {chain} {{ type nat hook {hook} priority -101; policy accept; }}")
        prefix = f'iifname "{interface}" ' if hook == "prerouting" else ""
        lines.append(f"add rule inet {TABLE} {chain} {prefix}ip daddr {host4} tcp dport {{ 80, 443 }} counter dnat ip to {target4}")
        for public, alternate in ((80, http6), (443, https6)):
            lines.append(f"add rule inet {TABLE} {chain} {prefix}ip6 daddr {host6} tcp dport {public} counter redirect to :{alternate}")
    return "\n".join(lines) + "\n"


def ipv6_input_rules(interface, host4, host6, target4, http6, https6):
    interface, _, host6, _, http6, https6 = validate(interface, host4, host6, target4, http6, https6)
    # Permit only translated public traffic. Direct alternate-port access stays
    # subject to the existing default-deny firewall. Established flows use the
    # existing stateful rule, including while the temporary NAT table is removed.
    return [["-i", interface, "-p", "tcp", "-d", host6, "--dport", str(alternate),
             "-m", "conntrack", "--ctstate", "DNAT",
             "--ctorigdst", host6, "--ctorigdstport", str(public), "-j", "ACCEPT"]
            for public, alternate in ((80, http6), (443, https6))]
