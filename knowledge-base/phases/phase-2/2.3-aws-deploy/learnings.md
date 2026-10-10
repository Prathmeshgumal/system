# Part 2.3 — First AWS deploy: Learnings

Spec: [design.md § 4](../design.md#section-4--part-23-first-aws-deploy) · Decision: [ADR 0005](../../../adr/0005-single-instance-deploy.md) ·
Concept: [Note 11](../../../Notes.md#11-one-server-on-the-internet-vpc-firewall-iam-tls) · Runbook: [deploy/README.md](../../../../deploy/README.md)

## What was built

| Piece | Where |
|---|---|
| Production stack: Caddy (HTTPS, CSP), API, Postgres, monitoring | `deploy/compose.prod.yaml`, `web/Caddyfile.prod` |
| Optional on-server storage (no S3 needed) | `deploy/compose.storage.yaml`, `deploy/storage.caddy` |
| `make deploy`: build → ship over SSH → start → HTTPS smoke check; rollback by SHA | `scripts/deploy.sh` |
| Terraform: VPC, subnet, SG, key, EC2 (standard credits, IMDSv2), EIP, S3 + CORS, IAM role, budget | `deploy/terraform/` |
| Settings template | `deploy/.env.prod.example` |

## What was verified, and how

There were no AWS credentials in the sandbox this was built in, and Terraform's provider
downloads were blocked. So:

| Verified | How |
|---|---|
| `make deploy` end to end | against the build machine itself over SSH (`DOMAIN=localhost`, Caddy's local CA, RustFS storage) |
| The deployed stack works | all 7 Playwright tests over HTTPS against it, with the CSP on |
| Rollback | deployed a second release, then `RELEASE=<first>` again |
| Headers | HSTS, CSP, `nosniff`, HTTP→HTTPS 308, HTTP/3 advertised |
| Terraform syntax and references | `hclfmt` (HashiCorp's HCL parser) + a script that checks every `resource.name`, `var.` and `local.` is defined |
| **Not verified** | `terraform plan/apply`, real Let's Encrypt, S3 via the IAM role, the user-data script — **yours to run** |

## Already observed while building

- **Shared volume names bite.** The first local deploy failed with `password authentication failed`:
  the prod stack's project was also called `relay`, so it reused full mode's `relay_pgdata`
  volume — and Postgres only reads `POSTGRES_PASSWORD` when a volume is new. The prod project is
  now `relay-prod`.
- **`ulimit` on WebSockets:** every connection is an open file; Docker's default limit (1024)
  caps users long before CPU does. The API container asks for 65,536 (configurable: this
  sandbox's Docker only allowed 20,000).
- **Caddy needs no certificate setup at all:** with `DOMAIN=localhost` it made its own CA and
  certificate; with a public name it uses Let's Encrypt the same way.
- **Docker Hub rate limits** (anonymous pulls) stopped image builds for a while: base images that
  only live in BuildKit's cache still get re-checked against the registry. On your machine this
  is unlikely; if it happens, `docker login` raises the limit.
- Memory, measured: Caddy ~20 MiB, API ~9 MiB idle, Postgres ~37 MiB, Prometheus ~34, Loki ~44,
  Alloy ~42, Grafana ~230 (OOM-killed at a 192 MiB limit). The production set idles at ~415 MiB.

## Your first deploy — by hand first, then Terraform

The roadmap says: click through it once, then automate. In the AWS console (your region):

1. **Billing → Budgets → Create budget → "Zero spend budget"** with your e-mail. Do this first.
2. **VPC → Create VPC → "VPC only"**, 10.0.0.0/16. Then a subnet 10.0.1.0/24, an internet gateway
   (attach it), a route table with `0.0.0.0/0 → igw` associated with the subnet. Notice: no
   route = no internet, however open the firewall.
3. **EC2 → Security groups**: inbound 22 from *My IP*, 80 and 443 from anywhere.
4. **EC2 → Launch instance**: Ubuntu 24.04, t3.micro, your key pair, the subnet and SG.
   *Advanced → Credit specification: Standard.* *Metadata: V2 only, hop limit 2.*
5. **Elastic IP → Allocate → Associate** with the instance. SSH in: `ssh ubuntu@<ip>`.
6. Look around (`free -m`, `nproc`, `df -h`), then **terminate everything** and run
   `make infra-apply`: the same things, as code. Compare with `terraform state list`.

## Experiments to run (on AWS)

1. **Deploy:** follow [deploy/README.md](../../../../deploy/README.md). Time `make infra-apply`
   and `make deploy`. Open `https://<domain>` on your phone, chat with yourself.
2. **The firewall:** from a phone on mobile data (not your IP), `ssh` to the server. What
   happens, and how long until you get an answer? Why does it hang instead of refusing?
3. **IMDS:** on the server, `curl -s http://169.254.169.254/latest/meta-data/` (fails: v2 only),
   then with a token (`curl -X PUT …/api/token`). Now from inside the api container — then set the
   hop limit to 1 and upload a picture. Read the API log.
4. **Credits:** CloudWatch → EC2 → `CPUCreditBalance` during the load tests (Part 2.4).
5. **Rebuild from nothing:** `make infra-destroy && make infra-apply && make deploy`. How long?

**Your results:**
