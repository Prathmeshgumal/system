# Ubuntu 24.04, newest image from Canonical's official account.
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
}

resource "aws_key_pair" "deploy" {
  key_name   = "relay-deploy"
  public_key = file(pathexpand(var.ssh_public_key_path))
}

resource "aws_instance" "app" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.app.id]
  key_name               = aws_key_pair.deploy.key_name
  iam_instance_profile   = aws_iam_instance_profile.app.name
  user_data              = file("${path.module}/user_data.sh")

  # t3 instances default to "unlimited" CPU credits: sustained load past the
  # baseline keeps running fast — and is billed. "standard" throttles instead.
  # For a $0 budget, throttling is the right failure mode.
  credit_specification {
    cpu_credits = "standard"
  }

  # IMDSv2 only (token required): blocks the classic SSRF trick of making the app
  # fetch http://169.254.169.254/... to steal the instance's credentials.
  # Hop limit 2: containers are one network hop further away than the host, and
  # the API needs the IAM role's credentials for S3.
  metadata_options {
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.disk_gb
    encrypted   = true
  }

  # A newer Ubuntu image appears every few weeks; don't replace the server
  # (and its database!) just because of that.
  lifecycle {
    ignore_changes = [ami, user_data]
  }

  tags = { Name = "relay-app" }
}

# A fixed public IP that survives stop/start, so DNS and certificates keep working.
# Free while attached to a running instance (within the free tier's 750 h of public IPv4).
resource "aws_eip" "app" {
  instance = aws_instance.app.id
  domain   = "vpc"
  tags     = { Name = "relay-app" }
}

locals {
  # No domain? <ip with dashes>.sslip.io resolves to that IP — a free name that
  # Let's Encrypt will issue a certificate for.
  domain = var.domain != "" ? var.domain : "${replace(aws_eip.app.public_ip, ".", "-")}.sslip.io"
}
