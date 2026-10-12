---
name: Feature request
about: Suggest an idea for Sentra
title: "[feature] "
labels: enhancement
---

## What do you need?

A clear and concise description of the feature you want.

## Why do you need it?

What problem does this solve for you? What are you trying to do?

## How should it work?

A rough idea of the expected behavior, command syntax, or Sentrafile
directive.

## Have you considered alternatives?

Any workarounds you've tried or other approaches you've considered.

## Scope check

Sentra has hard scope limits (see [AGENT.md](AGENT.md)). Please confirm this
request does not fall under the explicitly out-of-scope items:

- [ ] CRIU snapshot/restore
- [ ] eBPF-based auto-seccomp or live metrics
- [ ] AI-powered features (`sentra fix`, `sentra optimize`, etc.)
- [ ] Built-in micro-distro synthesizer
- [ ] Multiple network drivers or storage backends
- [ ] GPU support
- [ ] cgroups v1 support

If your request touches one of these, it will be redirected to the roadmap
rather than implemented in the MVP.
