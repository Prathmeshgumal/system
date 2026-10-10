# Deploying Relay to AWS — runbook

Everything for Part 2.3. Background and decisions: [phase 2 design](../knowledge-base/phases/phase-2/design.md#section-4--part-23-first-aws-deploy),
[ADR 0005](../knowledge-base/adr/0005-single-instance-deploy.md), and the console walkthrough in
[2.3 learnings](../knowledge-base/phases/phase-2/2.3-aws-deploy/learnings.md).

```
make infra-apply  ──►  EC2 t3.micro + Elastic IP + S3 bucket + IAM role + budget alert
make deploy       ──►  build images → ship over SSH → docker compose up → HTTPS smoke check
```

## 0. Before anything touches AWS (once, by hand)

1. **Root user:** turn on MFA. Never use root again after step 2.
2. **An admin IAM user** (or IAM Identity Center user) for yourself, with MFA. Create an
   access key for it and run `aws configure` (Terraform uses those credentials).
3. **Check which free tier you have** (Billing → Free tier): the 6-month *Free plan*
   (credits; AWS won't charge your card) or the legacy 12-month free tier (charges past the limits).
4. Install Terraform ≥ 1.10 and the AWS CLI.

## 1. First time: create the infrastructure

```sh
ssh-keygen -t ed25519 -f ~/.ssh/relay_ed25519 -C relay-deploy  # the key you'll deploy with
cp deploy/terraform/terraform.tfvars.example deploy/terraform/terraform.tfvars
#   → set ssh_cidr (your IP /32) and alert_email
make infra-init
make infra-plan     # read it: ~23 resources, nothing that costs money
make infra-apply    # ~1 minute; then wait ~2 more for the server to install Docker
```

Confirm the budget alert e-mail AWS sends you. Then:

```sh
cp deploy/.env.prod.example deploy/.env.prod
make infra-env      # paste these lines into deploy/.env.prod
#   → also fill ACME_EMAIL and the three secrets (openssl rand -base64 32 for each)
```

## 2. Deploy

```sh
make deploy
```

It builds `relay-api:<git sha>` and `relay-web:<git sha>` for linux/amd64, ships them,
starts them, and finishes with `✅ deployed <sha> to https://<domain>`. The first deploy
also gets the Let's Encrypt certificate (a few seconds).

| | |
|---|---|
| Deploy what's committed | `make deploy` (warns about uncommitted changes) |
| Roll back | `make deploy RELEASE=<older sha>` — reuses the images built back then |
| What's running? | `ssh … cat relay/RELEASE` |
| Logs | Grafana → Explore → Loki, or `ssh … 'cd relay && docker compose -f compose.prod.yaml logs -f api'` |
| Grafana | `terraform -chdir=deploy/terraform output -raw grafana_tunnel` → run it → http://localhost:3000 (user `admin`) |
| Database shell | `ssh … 'docker exec -it relay-prod-postgres-1 psql -U relay'` |

## 3. Every lab session

- **Done for the day and don't need it running?** `make infra-destroy`. Everything — the
  server, the database and the pictures — is deleted. `make infra-apply && make deploy`
  brings a fresh, empty one back in ~5 minutes.
- Check **Billing → Bills** once a week anyway. The zero-spend budget e-mails you at the first cent.

## Gotchas

| Symptom | Cause |
|---|---|
| `password authentication failed` after changing `POSTGRES_PASSWORD` | Postgres only reads it when the data volume is first created. Change it inside Postgres (`ALTER USER relay PASSWORD …`), or start over with a new volume |
| Certificate errors / Caddy logs "rate limited" | Let's Encrypt limits certificates per domain per week. Don't delete the `caddy_data` volume casually |
| Picture upload fails in the browser console with a CSP or CORS error | `S3_UPLOAD_ORIGIN` must exactly match the bucket's URL, and the bucket CORS rule must list `https://<domain>` — both come from `make infra-env` |
| `403` from S3 in the API logs | The API can't get the instance role's credentials: the metadata hop limit must be 2 (Terraform sets it) |
| Everything slow after a busy hour | CPU credits ran out: the instance is throttled to its 20 % baseline. Grafana → API CPU; AWS console → CPU credit balance |
| `error setting rlimit` when starting the api | Your Docker can't raise open files to 65536; set `API_MAX_OPEN_FILES` lower in `.env.prod` |

## Not on AWS?

Any Linux server with Docker works (Oracle Cloud's always-free Arm VM, a VPS, a Raspberry Pi):
set `LOCAL_STORAGE=true` and S3 keys of your choice in `.env.prod`, and pictures are stored in
RustFS on the server instead of S3. For an Arm server, build with `--platform linux/arm64`
(edit `scripts/deploy.sh`).
