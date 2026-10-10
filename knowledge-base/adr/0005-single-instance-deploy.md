# 0005 — First deploy: one EC2 instance, Docker Compose, images shipped over SSH

- **Status:** accepted
- **Date:** 2026-10-10
- **Phase:** 2 (Part 2.3)

## Context
v0.2 must be reachable at a real HTTPS URL, rebuilt by one command, at $0. One user group,
~100 users; Phase 3 adds more servers. No CI yet (no GitHub Actions in the plan until GitHub
joins). The goal is to learn the AWS basics by hand, then as code.

## Options considered
1. **ECS / App Runner / Elastic Beanstalk** — managed, but they hide exactly what we want to
   learn (networking, the instance, the proxy) and some parts aren't free (load balancer).
2. **RDS for Postgres** — free tier micro instance for 12 months; but a second resource to
   secure and network, and Postgres in a container is fine for 100 users.
3. **One EC2 t3.micro with Docker Compose** (Caddy + API + Postgres + monitoring) — everything
   visible, same compose model as full mode, free tier.

**Shipping code:** (a) build on the server — a 1 GiB instance compiling Go and Vite runs out of
memory; (b) a registry (ECR/GHCR) — another account and credentials; (c) **`docker save | ssh
docker load`** — nothing new to secure, ~40 MB per deploy.

**Name & certificate:** a real domain costs ~$10/year; **`<ip>.sslip.io`** resolves to the IP for
free and Let's Encrypt issues certificates for it; Caddy automates issuance and renewal.

## Decision
Terraform creates a VPC, one public subnet, a security group (443/80 from anywhere, 22 from your
IP), an Ubuntu t3.micro with an Elastic IP, a private S3 bucket and an IAM role for it, and a
zero-spend budget alert. `make deploy` builds images tagged with the git SHA, ships them over SSH,
copies the compose files and `.env` (chmod 600), runs `docker compose up -d --wait`, and smoke-tests
over HTTPS. Rollback = deploy an older SHA.

Guard-rails: **`cpu_credits = "standard"`** (t3 defaults to *unlimited*, which bills sustained
CPU), IMDSv2 only with hop limit 2 (containers need the role credentials), encrypted gp3 20 GB,
no NAT gateway, no load balancer, 2 GiB swap, Docker log rotation, everything tagged `project=relay`.

## Consequences
- ✅ `make infra-apply && make deploy` rebuilds everything; `make infra-destroy` removes it.
- ✅ Deploys are reproducible by SHA; no registry, no stored cloud credentials on the server.
- ❌ A deploy restarts the API: WebSockets drop for a few seconds (clients reconnect and catch up).
  Zero-downtime deploys need two instances behind a load balancer — Phase 3.4.
- ❌ Postgres, the app and monitoring share one small box: a runaway one starves the others.
  Memory limits per container contain it; real separation is Phase 3.
- ❌ Backups: none yet. The EBS volume dies with `terraform destroy`. Phase 3.4 adds backups and
  a tested restore.
- ❌ "standard" credits mean that sustained load beyond 20 % of 2 vCPUs throttles hard once the
  credit balance is spent — measured in the capacity write-up.
