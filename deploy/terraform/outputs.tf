output "public_ip" {
  value = aws_eip.app.public_ip
}

output "domain" {
  value = local.domain
}

output "ssh" {
  value = "ssh -i ${trimsuffix(var.ssh_public_key_path, ".pub")} ubuntu@${aws_eip.app.public_ip}"
}

output "grafana_tunnel" {
  description = "Grafana listens on the server's 127.0.0.1 only: run this, then open http://localhost:3000"
  value       = "ssh -i ${trimsuffix(var.ssh_public_key_path, ".pub")} -N -L 3000:127.0.0.1:3000 ubuntu@${aws_eip.app.public_ip}"
}

# Paste into deploy/.env.prod (`make infra-env` prints just this).
output "env_prod" {
  value = <<-EOT
    DEPLOY_HOST=${aws_eip.app.public_ip}
    DEPLOY_USER=ubuntu
    DEPLOY_SSH_KEY=${trimsuffix(var.ssh_public_key_path, ".pub")}
    DOMAIN=${local.domain}
    S3_BUCKET=${aws_s3_bucket.media.bucket}
    S3_REGION=${var.region}
    S3_UPLOAD_ORIGIN=https://${aws_s3_bucket.media.bucket_regional_domain_name}
  EOT
}
