# Phase 2 — Live on the internet (v0.2)

**Goal:** friends anywhere use Relay at a real HTTPS URL; it feels alive (typing, receipts,
reactions, replies, notifications, pictures, dark mode); and you can see how the server is doing.

**Docs:** [design](design.md) · [build plan](plan.md) · ADRs [0004 storage](../../adr/0004-object-storage-uploads.md),
[0005 deploy](../../adr/0005-single-instance-deploy.md), [0006 observability](../../adr/0006-observability.md) ·
Notes [8](../../Notes.md#8-temporary-vs-stored-events-and-write-amplification), [9](../../Notes.md#9-metrics-red-use-and-label-cardinality),
[10](../../Notes.md#10-object-storage--presigned-uploads), [11](../../Notes.md#11-one-server-on-the-internet-vpc-firewall-iam-tls) ·
Runbook [deploy/README.md](../../../deploy/README.md) · Capacity [one micro instance](../../concepts/capacity-one-micro-instance.md)

| Part | Build | Learn | Status | Docs |
|---|---|---|---|---|
| 2.1 Richer chat | Typing, read receipts, reactions, replies, notifications + title badge, dark mode | Temporary vs stored events, write amplification | ✅ built — experiments are yours | [learnings](2.1-richer-chat/learnings.md) |
| 2.2 Media | Profile pictures: presigned POST to S3 / RustFS, served from `/media` | Object storage, presigned URLs, why files don't go in the DB | ✅ built — experiments are yours | [learnings](2.2-media/learnings.md) |
| 2.3 First AWS deploy | Terraform (VPC, EC2, S3, IAM, budget), production compose, Caddy HTTPS + CSP, `make deploy` | Networking basics, DNS, TLS, IaC | 🟨 built and tested locally over SSH — **`terraform apply` + first real deploy are yours** | [learnings](2.3-aws-deploy/learnings.md) |
| 2.4 Observability | Prometheus metrics, JSON logs → Loki, provisioned Grafana dashboard, soak + breakpoint tests | RED/USE, cardinality, capacity limits | ✅ built and measured on a t3.micro-sized local stack — **AWS run is yours** | [learnings](2.4-observability/learnings.md) |

**Ends with:** v0.2 — see [01-app-versions.md](../../01-app-versions.md#v02--live-on-the-internet-end-of-phase-2).
