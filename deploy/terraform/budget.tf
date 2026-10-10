# Zero-spend budget: an email as soon as the account spends a single cent.
# The first two budgets are free. This is the most important resource here.
resource "aws_budgets_budget" "zero_spend" {
  name         = "relay-zero-spend"
  budget_type  = "COST"
  limit_amount = "1.00"
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 0.01
    threshold_type             = "ABSOLUTE_VALUE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.alert_email]
  }
}
