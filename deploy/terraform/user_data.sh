#!/bin/bash
# Runs once, as root, on the instance's first boot (cloud-init).
# Log: /var/log/cloud-init-output.log
set -euxo pipefail

# 2 GiB of swap: 1 GiB of RAM is tight for Postgres + API + Caddy + monitoring.
# Swap turns "out of memory, killed" into "a bit slower".
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
echo 'vm.swappiness=10' > /etc/sysctl.d/99-relay.conf # prefer RAM; swap only under pressure
sysctl --system

# Docker and Compose from Ubuntu's own packages.
apt-get update
apt-get install -y docker.io docker-compose-v2

# Cap container logs (Loki keeps the history), or they fill the disk eventually.
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }
JSON
systemctl enable --now docker
systemctl restart docker
usermod -aG docker ubuntu # the deploy user runs docker without sudo

# Security updates install themselves (unattended-upgrades is on by default in Ubuntu).
