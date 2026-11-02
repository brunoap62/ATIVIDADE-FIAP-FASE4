resource "aws_sqs_queue" "terraform_queue" {
  name                      = var.sqs_name
  delay_seconds             = 0
  max_message_size          = 2048
  message_retention_seconds = 86400
  receive_wait_time_seconds = 10
  sqs_managed_sse_enabled   = true

  tags = {
    Iac = true
  }
}

