---
title: sikifanso
hide:
  - navigation
  - toc
---

<p align="center">
  <img src="assets/logo.png" alt="sikifanso" width="200">
</p>

<h1 align="center" style="font-family: 'Space Grotesk', sans-serif;">sikifanso</h1>

<p align="center">
  <strong>Bootstrap a fully functional homelab Kubernetes cluster with a single command.</strong>
</p>

<p align="center">
  <a href="https://github.com/sikifanso/sikifanso/releases/latest"><img src="https://img.shields.io/github/v/release/sikifanso/sikifanso?color=blue" alt="Release"></a>
  <a href="https://github.com/sikifanso/sikifanso/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/sikifanso/sikifanso/ci.yml?label=CI" alt="CI"></a>
  <a href="https://github.com/sikifanso/sikifanso/blob/main/LICENSE"><img src="https://img.shields.io/github/license/sikifanso/sikifanso" alt="License"></a>
  <a href="https://goreportcard.com/report/github.com/sikifanso/sikifanso"><img src="https://goreportcard.com/badge/github.com/sikifanso/sikifanso" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/sikifanso/sikifanso"><img src="https://img.shields.io/badge/go-1.25+-00ADD8?logo=go" alt="Go version"></a>
  <a href="https://sikifanso.com"><img src="https://img.shields.io/badge/docs-sikifanso.com-3F9AAE" alt="Docs"></a>
</p>

<p align="center">
  <img src="assets/demo.gif" alt="demo" width="800">
</p>

---

## What you get

```bash
sikifanso cluster create
```

- **k3d cluster** — 1 server + 2 agents, k3s v1.29
- **Cilium** — full kube-proxy replacement, ingress controller, Hubble UI
- **ArgoCD** — configured to read from a local gitops repo on your filesystem
- **GitOps repo** — scaffolded from a bootstrap template, mounted into the cluster
- **Root ApplicationSet** — watches `apps/coordinates/*.yaml` and deploys automatically

No remote git server. No cloud account. Just Docker and a single command.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (running)

That's it. You do **not** need to install k3d, Helm, Cilium, ArgoCD, or any other Kubernetes tooling — sikifanso embeds everything and handles the full stack internally.

## Install

=== "Homebrew"

    ```bash
    brew install --cask sikifanso/tap/sikifanso
    ```

=== "Go"

    ```bash
    go install github.com/sikifanso/sikifanso/cmd/sikifanso@latest
    ```

=== "From source"

    ```bash
    git clone https://github.com/sikifanso/sikifanso.git
    cd sikifanso
    go build -o sikifanso ./cmd/sikifanso
    ```

---

<p align="center">
  <a href="getting-started/" class="md-button md-button--primary">Get Started</a>
  <a href="architecture/" class="md-button">How It Works</a>
</p>
