# SPDK CSI

## About

This repo contains SPDK CSI ([Container Storage Interface]((https://github.com/container-storage-interface/)) plugin for Kubernetes.

SPDK CSI plugin brings SPDK to Kubernetes. It provisions SPDK logical volumes on storage node dynamically and enables Pods to access SPDK storage backend through NVMe-oF or iSCSI.

Please see [SPDK CSI Design Document](https://docs.google.com/document/d/1aLi6SkNBp__wjG7YkrZu7DdhoftAquZiWiIOMy3hskY/) for detailed introduction.

## Supported platforms

This plugin conforms to [CSI Spec v1.1.0](https://github.com/container-storage-interface/spec/blob/v1.1.0/spec.md). It is currently developed and tested only on Kubernetes.

This plugin supports `x86_64` and `Arm64` architectures.

## Project status

Status: **Pre-alpha**

## Prerequisites

SPDK-CSI is currently developed and tested with `Go 1.14`, `Docker 19.03` and `Kubernetes 1.17`.

Minimal requirement: Go 1.12+(supports Go module), Docker 18.03+ and Kubernetes 1.13+(supports CSI spec 1.0).

## Setup

### Build

- `$ make all`
Build targets spdkcsi, lint, test.

- `$ make spdkcsi`
Build SPDK-CSI binary `_out/spdkcsi`.

- `$ make lint`
Lint code and scripts.
  - `$ make golangci`
Install [golangci-lint](https://github.com/golangci/golangci-lint) and perform various go code static checks.

- `$ make test`
Verify go modules and run unit tests.

### Parameters

`spdkcsi` executable accepts several command line parameters.

| Parameter      | Type   | Description               | Default           |
| ---------      | ----   | -----------               | -------           |
| `--controller` | -      | enable controller service | -                 |
| `--node`       | -      | enable node service       | -                 |
| `--endpoint`   | string | communicate with sidecars | /tmp/spdkcsi.sock |
| `--drivername` | string | driver name               | csi.spdk.io       |
| `--nodeid`     | string | node id                   | -                 |

## Usage

TODO

## Communication and Contribution

Please join [SPDK community](https://spdk.io/community/) for communication and contribution.

Project progress is tracked in [Trello board](https://trello.com/b/nBujJzya/kubernetes-integration).
