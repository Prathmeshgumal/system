variable "region" {
  description = "AWS region. Pick one close to your users; prices and free tier are the same."
  type        = string
  default     = "eu-central-1"
}

variable "instance_type" {
  description = "t3.micro is free-tier eligible (750 h/month = one instance all month)."
  type        = string
  default     = "t3.micro"
}

variable "ssh_public_key_path" {
  description = "Public half of the key you'll SSH and deploy with (ssh-keygen -t ed25519 -f ~/.ssh/relay_ed25519)."
  type        = string
  default     = "~/.ssh/relay_ed25519.pub"
}

variable "ssh_cidr" {
  description = "Who may SSH in: your public IP as a /32 (curl -s https://checkip.amazonaws.com)."
  type        = string
  validation {
    condition     = can(cidrhost(var.ssh_cidr, 0)) && var.ssh_cidr != "0.0.0.0/0"
    error_message = "Use your own IP, e.g. 203.0.113.7/32 — never 0.0.0.0/0 for SSH."
  }
}

variable "alert_email" {
  description = "Where the zero-spend budget alert goes."
  type        = string
}

variable "domain" {
  description = "Your own domain (A record → the Elastic IP). Empty: a free <ip>.sslip.io name."
  type        = string
  default     = ""
}

variable "disk_gb" {
  description = "Root disk size. The free tier includes 30 GB of gp3/gp2 in total."
  type        = number
  default     = 20
}
