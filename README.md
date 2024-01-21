# kindops

A simple package to make a fully functional kind cluster in a Linux(Ubuntu) 
machine with Load Balancer, Volume support, helm chart installation support, 
kubectl like apply of yaml support etc.

This will come handy for running E2E for your app in a linux machine, 
especially a VM running in your laptop

## Test info 

### OS

Ubuntu 22.04.2 LTS in Windows Hypervisor runs Windows 11. VM is given 16GB mem 
and 4 vCPUs.

Ideally it should in any linux though I have not tested.

### Golang

go1.20.7 linux/amd64

### Kind

kind v0.20.0 go1.20.7 linux/amd64
https://kind.sigs.k8s.io/docs/user/quick-start/

## Running instructions

Check the main file in this directory for how to use this.
Configs used in config directory can be referred. Especially 
"kind_test_cluster_config.yaml".
